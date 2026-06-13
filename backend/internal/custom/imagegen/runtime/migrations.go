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
	// DefaultMigrationsDir 是 tasks-3 薄接入可直接复用的 custom SQL 默认目录。
	DefaultMigrationsDir = "backend/migrations/custom/imagegen"
	// DefaultMigrationsTable 与主仓 schema_migrations 隔离，避免 custom SQL 占用主迁移编号。
	DefaultMigrationsTable = "custom_imagegen_schema_migrations"
)

const migrationsTableDDL = `
CREATE TABLE IF NOT EXISTS %s (
	filename TEXT PRIMARY KEY,
	checksum TEXT NOT NULL,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

// MigrationOptions 控制 custom 生图 SQL runner 的迁移表名和表前缀替换。
type MigrationOptions struct {
	MigrationsTable string
	TablePrefix     string
}

// ApplyMigrationsDir 从磁盘目录读取 custom 生图 SQL 并执行。
//
// 该 runner 不扫描主仓 backend/migrations/*.sql；调用方必须显式传入
// backend/migrations/custom/imagegen，避免和主仓迁移编号、校验表发生耦合。
func ApplyMigrationsDir(ctx context.Context, db *sql.DB, dir string, opts MigrationOptions) error {
	if strings.TrimSpace(dir) == "" {
		dir = DefaultMigrationsDir
	}
	return ApplyMigrationsFS(ctx, db, os.DirFS(dir), opts)
}

// ApplyMigrationsFS 按文件名顺序执行 fsys 根目录下的 *.sql。
//
// 每个文件在独立事务中执行，并写入独立迁移表；已应用文件会校验 checksum，
// 防止历史 custom migration 被静默改写。
func ApplyMigrationsFS(ctx context.Context, db *sql.DB, fsys fs.FS, opts MigrationOptions) error {
	if db == nil {
		return errors.New("nil sql db")
	}
	if err := validateIdentifierPart(opts.TablePrefix, true); err != nil {
		return fmt.Errorf("custom imagegen table prefix is invalid: %w", err)
	}
	tableName, err := safeIdentifier(defaultString(opts.MigrationsTable, DefaultMigrationsTable))
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(migrationsTableDDL, tableName)); err != nil {
		return fmt.Errorf("create custom imagegen migrations table: %w", err)
	}

	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return fmt.Errorf("list custom imagegen migrations: %w", err)
	}
	sort.Strings(files)
	for _, name := range files {
		contentBytes, err := fs.ReadFile(fsys, filepath.ToSlash(name))
		if err != nil {
			return fmt.Errorf("read custom imagegen migration %s: %w", name, err)
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
				return fmt.Errorf("custom imagegen migration %s checksum mismatch", filename)
			}
			continue
		}
		if !errors.Is(rowErr, sql.ErrNoRows) {
			return fmt.Errorf("check custom imagegen migration %s: %w", filename, rowErr)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin custom imagegen migration %s: %w", filename, err)
		}
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply custom imagegen migration %s: %w", filename, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+tableName+" (filename, checksum) VALUES ($1, $2)", filename, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record custom imagegen migration %s: %w", filename, err)
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit custom imagegen migration %s: %w", filename, err)
		}
	}
	return nil
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
