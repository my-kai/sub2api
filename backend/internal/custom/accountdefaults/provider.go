package accountdefaults

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	accountdefaultsmigrations "github.com/Wei-Shaw/sub2api/migrations/custom/accountdefaults"
	"github.com/google/wire"
)

// migrationAdvisoryLockID serializes account-defaults schema updates across instances.
const migrationAdvisoryLockID int64 = 2026092511

// Bundle groups the custom account-defaults runtime and administrator handler.
type Bundle struct {
	Service *Service
	Handler *Handler
}

// ProvideBundle applies the isolated schema and wires the runtime.
func ProvideBundle(db *sql.DB, proxies service.ProxyRepository) (*Bundle, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := applyMigrations(ctx, db, accountdefaultsmigrations.FS); err != nil {
		return nil, err
	}
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	svc, err := NewService(store, proxies)
	if err != nil {
		return nil, err
	}
	return &Bundle{Service: svc, Handler: NewHandler(svc)}, nil
}

// ProviderSet binds custom account-defaults dependencies for Wire.
var ProviderSet = wire.NewSet(ProvideBundle)

func applyMigrations(ctx context.Context, db *sql.DB, source fs.FS) error {
	if db == nil || source == nil {
		return fmt.Errorf("account defaults migration dependencies are required")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open account defaults migration connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("lock account defaults migrations: %w", err)
	}
	defer func() {
		if _, unlockErr := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLockID); unlockErr != nil {
			_ = unlockErr
		}
	}()
	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS custom_account_defaults_schema_migrations (
			filename TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create account defaults migrations table: %w", err)
	}
	names, err := fs.Glob(source, "*.sql")
	if err != nil {
		return fmt.Errorf("list account defaults migrations: %w", err)
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := fs.ReadFile(source, name)
		if err != nil {
			return fmt.Errorf("read account defaults migration %s: %w", name, err)
		}
		sum := sha256.Sum256(raw)
		checksum := hex.EncodeToString(sum[:])
		var existing string
		rowErr := conn.QueryRowContext(ctx, `SELECT checksum FROM custom_account_defaults_schema_migrations WHERE filename = $1`, name).Scan(&existing)
		switch {
		case rowErr == nil:
			if existing != checksum {
				return fmt.Errorf("account defaults migration %s checksum mismatch", name)
			}
			continue
		case !errors.Is(rowErr, sql.ErrNoRows):
			return fmt.Errorf("check account defaults migration %s: %w", name, rowErr)
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin account defaults migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(raw)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply account defaults migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO custom_account_defaults_schema_migrations (filename, checksum) VALUES ($1, $2)`, name, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record account defaults migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit account defaults migration %s: %w", name, err)
		}
	}
	return nil
}
