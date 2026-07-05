package invoice

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

	invoicemigrations "github.com/Wei-Shaw/sub2api/migrations/custom/invoice"
)

const defaultMigrationTimeout = 10 * time.Second
const invoiceMigrationAdvisoryLockID int64 = 2026070501

// Bundle holds custom invoice runtime dependencies for thin server wiring.
type Bundle struct {
	Handler *Handler
	Service *Service
	Store   *Store
	Files   *FileStore
}

// ProviderOptions lets tests and server wiring override invoice runtime dependencies.
type ProviderOptions struct {
	DataDir                string
	MigrationsFS           fs.FS
	Timeout                time.Duration
	EmailSender            EmailSender
	PublicDownloadBaseURL  string
	PublicDownloadTokenKey string
}

// ProvideBundle initializes invoice storage, migrations and handlers.
func ProvideBundle(db *sql.DB, dataDir string) (*Bundle, error) {
	return ProvideBundleWithOptions(db, ProviderOptions{DataDir: dataDir})
}

// ProvideBundleWithEmail initializes invoice storage and enables completion emails.
func ProvideBundleWithEmail(db *sql.DB, dataDir string, emailSender EmailSender, publicDownloadBaseURL, publicDownloadTokenKey string) (*Bundle, error) {
	return ProvideBundleWithOptions(db, ProviderOptions{
		DataDir:                dataDir,
		EmailSender:            emailSender,
		PublicDownloadBaseURL:  publicDownloadBaseURL,
		PublicDownloadTokenKey: publicDownloadTokenKey,
	})
}

// ProvideBundleWithOptions initializes invoice storage for tests and server wiring.
func ProvideBundleWithOptions(db *sql.DB, opts ProviderOptions) (*Bundle, error) {
	if db == nil {
		return nil, fmt.Errorf("sql db is required")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultMigrationTimeout
	}
	fsys := opts.MigrationsFS
	if fsys == nil {
		fsys = invoicemigrations.FS
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := applyMigrations(ctx, db, fsys); err != nil {
		return nil, err
	}
	store, err := NewStore(db)
	if err != nil {
		return nil, err
	}
	files, err := NewFileStore(opts.DataDir)
	if err != nil {
		return nil, err
	}
	service, err := NewServiceWithOptions(store, files, ServiceOptions{
		EmailSender:            opts.EmailSender,
		PublicDownloadBaseURL:  opts.PublicDownloadBaseURL,
		PublicDownloadTokenKey: opts.PublicDownloadTokenKey,
	})
	if err != nil {
		return nil, err
	}
	return &Bundle{Handler: NewHandler(service), Service: service, Store: store, Files: files}, nil
}

func applyMigrations(ctx context.Context, db *sql.DB, fsys fs.FS) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open custom invoice migration connection: %w", err)
	}
	defer conn.Close()

	// Custom migrations run at service startup. A PostgreSQL advisory lock keeps
	// multi-instance deployments from racing on the checksum table insert.
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, invoiceMigrationAdvisoryLockID); err != nil {
		return fmt.Errorf("lock custom invoice migrations: %w", err)
	}
	defer func() {
		if _, unlockErr := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, invoiceMigrationAdvisoryLockID); unlockErr != nil && err == nil {
			err = fmt.Errorf("unlock custom invoice migrations: %w", unlockErr)
		}
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS custom_invoice_schema_migrations (
			filename TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create custom invoice migrations table: %w", err)
	}
	files, err := fs.Glob(fsys, "*.sql")
	if err != nil {
		return fmt.Errorf("list custom invoice migrations: %w", err)
	}
	sort.Strings(files)
	for _, name := range files {
		contentBytes, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read custom invoice migration %s: %w", name, err)
		}
		content := strings.TrimSpace(string(contentBytes))
		if content == "" {
			continue
		}
		sum := sha256.Sum256([]byte(content))
		checksum := hex.EncodeToString(sum[:])
		var existing string
		rowErr := conn.QueryRowContext(ctx, "SELECT checksum FROM custom_invoice_schema_migrations WHERE filename = $1", name).Scan(&existing)
		if rowErr == nil {
			if existing != checksum {
				return fmt.Errorf("custom invoice migration %s checksum mismatch", name)
			}
			continue
		}
		if rowErr != sql.ErrNoRows {
			return fmt.Errorf("check custom invoice migration %s: %w", name, rowErr)
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin custom invoice migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, content); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply custom invoice migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO custom_invoice_schema_migrations (filename, checksum) VALUES ($1, $2)", name, checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record custom invoice migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("commit custom invoice migration %s: %w", name, err)
		}
	}
	return nil
}
