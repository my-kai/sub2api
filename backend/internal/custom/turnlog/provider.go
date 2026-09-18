package turnlog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	turnlogmigrations "github.com/Wei-Shaw/sub2api/migrations/custom/turnlog"
	"github.com/google/wire"
)

// migrationAdvisoryLockID serializes turn-log schema updates across instances.
const migrationAdvisoryLockID int64 = 2026091811

// Bundle groups the custom turn-log runtime and administrator handler.
type Bundle struct {
	Service *Service
	Handler *Handler
}

// Close stops the retention worker before process resources are released.
func (b *Bundle) Close() {
	if b != nil && b.Service != nil {
		b.Service.Close()
	}
}

// ProvideBundle applies the isolated schema and starts retention cleanup.
func ProvideBundle(db *sql.DB, accountRepo service.AccountRepository, gateway *service.OpenAIGatewayService, proxyRepo service.ProxyRepository) (*Bundle, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := applyMigrations(ctx, db, turnlogmigrations.FS); err != nil {
		return nil, err
	}
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	runtime, err := NewService(store, accountRepo, gateway, proxyRepo)
	if err != nil {
		return nil, err
	}
	return &Bundle{Service: runtime, Handler: NewHandler(runtime)}, nil
}

// ProviderSet binds custom turn-log dependencies for Wire.
var ProviderSet = wire.NewSet(ProvideBundle)

func applyMigrations(ctx context.Context, db *sql.DB, source fs.FS) error {
	if db == nil || source == nil {
		return fmt.Errorf("turn log migration dependencies are required")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open turn log migration connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("lock turn log migrations: %w", err)
	}
	defer func() {
		if _, unlockErr := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID); unlockErr != nil {
			slog.Error("turn_log_migration_unlock_failed", "error", unlockErr)
		}
	}()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS custom_turn_log_schema_migrations (filename TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`); err != nil {
		return fmt.Errorf("create turn log migration table: %w", err)
	}
	files, err := fs.Glob(source, "*.sql")
	if err != nil {
		return fmt.Errorf("list turn log migrations: %w", err)
	}
	sort.Strings(files)
	for _, name := range files {
		contentBytes, err := fs.ReadFile(source, name)
		if err != nil {
			return fmt.Errorf("read turn log migration %s: %w", name, err)
		}
		content := strings.TrimSpace(string(contentBytes))
		if content == "" {
			continue
		}
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])
		var existing string
		rowErr := conn.QueryRowContext(ctx, `SELECT checksum FROM custom_turn_log_schema_migrations WHERE filename = $1`, name).Scan(&existing)
		if rowErr == nil {
			if existing != checksum {
				return fmt.Errorf("turn log migration %s checksum mismatch", name)
			}
			continue
		}
		if rowErr != sql.ErrNoRows {
			return fmt.Errorf("check turn log migration %s: %w", name, rowErr)
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin turn log migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply turn log migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO custom_turn_log_schema_migrations (filename, checksum) VALUES ($1, $2)`, name, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record turn log migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit turn log migration %s: %w", name, err)
		}
	}
	return nil
}
