package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	giftservice "github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/service"
	"github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/store"
	giftcreditmigrations "github.com/Wei-Shaw/sub2api/migrations/custom/giftcredit"
)

// Bundle is the runtime object later route/service tasks can receive from main wiring.
type Bundle struct {
	Store   *store.Store
	Service *giftservice.Service
}

// ProviderOptions lets tests and thin wiring override migration sources.
type ProviderOptions struct {
	TablePrefix     string
	MigrationsTable string
	MigrationsFS    fs.FS
}

// ProvideBundle applies custom gift-credit migrations and returns the SQL-backed store.
func ProvideBundle(ctx context.Context, db *sql.DB, opts ProviderOptions) (*Bundle, error) {
	if db == nil {
		return nil, fmt.Errorf("sql db is required")
	}
	if err := ValidateTablePrefix(opts.TablePrefix); err != nil {
		return nil, fmt.Errorf("validate custom gift credit table prefix: %w", err)
	}
	fsys := opts.MigrationsFS
	if fsys == nil {
		fsys = giftcreditmigrations.FS
	}
	if err := ApplyMigrationsFS(ctx, db, fsys, MigrationOptions{
		MigrationsTable: opts.MigrationsTable,
		TablePrefix:     opts.TablePrefix,
	}); err != nil {
		return nil, err
	}
	giftStore, err := store.NewStore(db, opts.TablePrefix)
	if err != nil {
		return nil, err
	}
	return &Bundle{Store: giftStore, Service: giftservice.NewService(giftStore)}, nil
}

// ProvideBundleFromEnv is the default entry for future main-server thin wiring.
func ProvideBundleFromEnv(db *sql.DB) (*Bundle, error) {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load custom gift credit config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MigrationTimeout)
	defer cancel()
	return ProvideBundle(ctx, db, ProviderOptions{TablePrefix: cfg.TablePrefix})
}
