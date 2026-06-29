package runtime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// DefaultMigrationsDir is the local development SQL directory for gift-credit migrations.
	DefaultMigrationsDir = "backend/migrations/custom/giftcredit"
	// DefaultMigrationsTable isolates gift-credit SQL from the upstream migration table.
	DefaultMigrationsTable = "custom_gift_credit_schema_migrations"
)

const migrationsTableDDL = `
CREATE TABLE IF NOT EXISTS %s (
	filename TEXT PRIMARY KEY,
	checksum TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

// MigrationOptions controls the custom gift-credit migration table and table-name prefix.
type MigrationOptions struct {
	MigrationsTable string
	TablePrefix     string
}

// ApplyMigrationsDir reads custom gift-credit SQL from a filesystem directory and executes it.
func ApplyMigrationsDir(ctx context.Context, db *sql.DB, dir string, opts MigrationOptions) error {
	if strings.TrimSpace(dir) == "" {
		dir = DefaultMigrationsDir
	}
	return ApplyMigrationsFS(ctx, db, os.DirFS(dir), opts)
}

// ApplyMigrationsFS executes *.sql files from fsys in filename order.
//
// Checksums are recorded so edited already-applied migration files fail fast
// instead of silently drifting production schema.
func ApplyMigrationsFS(ctx context.Context, db *sql.DB, fsys fs.FS, opts MigrationOptions) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	if err := ValidateTablePrefix(opts.TablePrefix); err != nil {
		return fmt.Errorf("custom gift credit table prefix is invalid: %w", err)
	}
	tableName, err := safeIdentifier(defaultString(opts.MigrationsTable, DefaultMigrationsTable))
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(migrationsTableDDL, tableName)); err != nil {
		return fmt.Errorf("create custom gift credit migrations table: %w", err)
	}
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return fmt.Errorf("list custom gift credit migrations: %w", err)
	}
	sort.Strings(files)
	for _, name := range files {
		contentBytes, err := fs.ReadFile(fsys, filepath.ToSlash(name))
		if err != nil {
			return fmt.Errorf("read custom gift credit migration %s: %w", name, err)
		}
		content := strings.TrimSpace(strings.ReplaceAll(string(contentBytes), "{{TABLE_PREFIX}}", opts.TablePrefix))
		if content == "" {
			continue
		}
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])
		filename := filepath.Base(name)
		var existing string
		rowErr := db.QueryRowContext(ctx, "SELECT checksum FROM "+tableName+" WHERE filename = $1", filename).Scan(&existing)
		if rowErr == nil {
			if existing != checksum {
				return fmt.Errorf("custom gift credit migration %s checksum mismatch", filename)
			}
			continue
		}
		if !errors.Is(rowErr, sql.ErrNoRows) {
			return fmt.Errorf("check custom gift credit migration %s: %w", filename, rowErr)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin custom gift credit migration %s: %w", filename, err)
		}
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply custom gift credit migration %s: %w", filename, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+tableName+" (filename, checksum) VALUES ($1, $2)", filename, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record custom gift credit migration %s: %w", filename, err)
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit custom gift credit migration %s: %w", filename, err)
		}
	}
	return nil
}

// ValidateTablePrefix checks the prefix before it is interpolated into SQL identifiers.
func ValidateTablePrefix(prefix string) error {
	return validateIdentifierPart(strings.TrimSpace(prefix), true)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func safeIdentifier(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("migration table name is required")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return "", fmt.Errorf("migration table name %q is invalid", raw)
	}
	return value, nil
}

func validateIdentifierPart(value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return errors.New("identifier is required")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return errors.New("identifier may only contain letters, digits or underscores")
	}
	return nil
}
