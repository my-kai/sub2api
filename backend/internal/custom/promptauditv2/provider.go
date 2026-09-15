package promptauditv2

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	promptauditv2migrations "github.com/Wei-Shaw/sub2api/migrations/custom/promptauditv2"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
)

const (
	// migrationTimeout bounds startup work for this isolated custom schema.
	migrationTimeout = 15 * time.Second
	// migrationAdvisoryLockID serializes this module's schema updates across instances.
	migrationAdvisoryLockID int64 = 2026090602
)

// Bundle groups custom dependencies used by the server route and shutdown seams.
type Bundle struct {
	Store   *Store
	Service *Service
	Handler *AdminHandler
}

// ProvideBundle applies isolated migrations and initializes the runtime and management handler.
func ProvideBundle(db *sql.DB, encryptor service.SecretEncryptor, redisClient *redis.Client, userService *service.UserService, emailService *service.EmailService) (*Bundle, error) {
	ctx, cancel := context.WithTimeout(context.Background(), migrationTimeout)
	defer cancel()
	if err := applyMigrations(ctx, db, promptauditv2migrations.FS); err != nil {
		return nil, err
	}
	store, err := NewStore(db, encryptor)
	if err != nil {
		return nil, err
	}
	healthStore, err := NewHealthStore(redisClient)
	if err != nil {
		return nil, err
	}
	runtime, err := NewService(store, healthStore, userService, emailService)
	if err != nil {
		return nil, err
	}
	return &Bundle{Store: store, Service: runtime, Handler: NewAdminHandler(runtime)}, nil
}

// ProvideService exposes the runtime separately for the security-audit coordinator.
func ProvideService(bundle *Bundle) *Service {
	if bundle == nil {
		return nil
	}
	return bundle.Service
}

// ProviderSet binds the custom runtime to the upstream coordinator's thin preflight contract.
var ProviderSet = wire.NewSet(
	ProvideBundle,
	ProvideService,
	wire.Bind(new(securityaudit.PreflightEngine), new(*Service)),
)

func applyMigrations(ctx context.Context, db *sql.DB, source fs.FS) (err error) {
	if db == nil || source == nil {
		return fmt.Errorf("prompt audit v2 migration dependencies are required")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open prompt audit v2 migration connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("lock prompt audit v2 migrations: %w", err)
	}
	defer func() {
		if _, unlockErr := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID); unlockErr != nil && err == nil {
			err = fmt.Errorf("unlock prompt audit v2 migrations: %w", unlockErr)
		}
	}()
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS custom_prompt_audit_v2_schema_migrations (
			filename TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create prompt audit v2 migration table: %w", err)
	}
	files, err := fs.Glob(source, "*.sql")
	if err != nil {
		return fmt.Errorf("list prompt audit v2 migrations: %w", err)
	}
	sort.Strings(files)
	for _, name := range files {
		contentBytes, err := fs.ReadFile(source, name)
		if err != nil {
			return fmt.Errorf("read prompt audit v2 migration %s: %w", name, err)
		}
		content := strings.TrimSpace(string(contentBytes))
		if content == "" {
			continue
		}
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])
		var existing string
		rowErr := conn.QueryRowContext(ctx, `SELECT checksum FROM custom_prompt_audit_v2_schema_migrations WHERE filename = $1`, name).Scan(&existing)
		if rowErr == nil {
			if existing != checksum {
				return fmt.Errorf("prompt audit v2 migration %s checksum mismatch (db=%s file=%s)", name, existing, checksum)
			}
			continue
		}
		if rowErr != sql.ErrNoRows {
			return fmt.Errorf("check prompt audit v2 migration %s: %w", name, rowErr)
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin prompt audit v2 migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply prompt audit v2 migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO custom_prompt_audit_v2_schema_migrations (filename, checksum) VALUES ($1, $2)`, name, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record prompt audit v2 migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit prompt audit v2 migration %s: %w", name, err)
		}
	}
	return nil
}
