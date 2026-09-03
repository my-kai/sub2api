package runtime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// defaultMigrationsTable isolates model-rate migration history from the upstream table.
const defaultMigrationsTable = "custom_model_rate_schema_migrations"

// ApplyMigrationsFS applies model-rate SQL with checksum protection.
func ApplyMigrationsFS(ctx context.Context, db *sql.DB, fsys fs.FS, tablePrefix, migrationsTable string) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	if strings.TrimSpace(migrationsTable) == "" {
		migrationsTable = defaultMigrationsTable
	}
	if err := validateIdentifier(migrationsTable); err != nil {
		return err
	}
	if err := validateIdentifier(tablePrefix); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (filename TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW())", migrationsTable)); err != nil {
		return err
	}
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, name := range files {
		contentBytes, err := fs.ReadFile(fsys, filepath.ToSlash(name))
		if err != nil {
			return err
		}
		content := strings.TrimSpace(strings.ReplaceAll(string(contentBytes), "{{TABLE_PREFIX}}", tablePrefix))
		if content == "" {
			continue
		}
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])
		var existing string
		err = db.QueryRowContext(ctx, "SELECT checksum FROM "+migrationsTable+" WHERE filename = $1", filepath.Base(name)).Scan(&existing)
		if err == nil {
			if existing != checksum {
				return fmt.Errorf("custom model rate migration %s checksum mismatch", filepath.Base(name))
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+migrationsTable+" (filename, checksum) VALUES ($1, $2)", filepath.Base(name), checksum); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func validateIdentifier(value string) error {
	for _, r := range strings.TrimSpace(value) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("invalid SQL identifier %q", value)
	}
	return nil
}
