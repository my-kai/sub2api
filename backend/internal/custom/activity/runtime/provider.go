package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	activityservice "github.com/Wei-Shaw/sub2api/internal/custom/activity/service"
	"github.com/Wei-Shaw/sub2api/internal/custom/activity/store"
	"github.com/Wei-Shaw/sub2api/internal/service"
	activitymigrations "github.com/Wei-Shaw/sub2api/migrations/custom/activity"
)

// Bundle is the runtime object later route/service tasks can receive from main wiring.
type Bundle struct {
	Store   *store.Store
	Service *activityservice.Service
}

// ProviderOptions lets tests and future thin wiring override migration sources.
type ProviderOptions struct {
	TablePrefix     string
	MigrationsTable string
	MigrationsFS    fs.FS
}

// ProvideBundle applies custom activity migrations and returns the SQL-backed store.
//
// The function intentionally performs only activity-owned setup. Main router,
// handler and balance-cache integration are handled by later tasks so this task
// does not expand into forbidden files.
func ProvideBundle(ctx context.Context, db *sql.DB, opts ProviderOptions) (*Bundle, error) {
	if db == nil {
		return nil, fmt.Errorf("sql db is required")
	}
	if err := ValidateTablePrefix(opts.TablePrefix); err != nil {
		return nil, fmt.Errorf("validate custom activity table prefix: %w", err)
	}
	fsys := opts.MigrationsFS
	if fsys == nil {
		fsys = activitymigrations.FS
	}
	if err := ApplyMigrationsFS(ctx, db, fsys, MigrationOptions{
		MigrationsTable: opts.MigrationsTable,
		TablePrefix:     opts.TablePrefix,
	}); err != nil {
		return nil, err
	}
	activityStore, err := store.NewStore(db, opts.TablePrefix)
	if err != nil {
		return nil, err
	}
	return &Bundle{Store: activityStore, Service: activityservice.NewService(activityStore)}, nil
}

// ProvideBundleFromEnv is the default entry for future main-server thin wiring.
func ProvideBundleFromEnv(db *sql.DB) (*Bundle, error) {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load custom activity config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MigrationTimeout)
	defer cancel()
	return ProvideBundle(ctx, db, ProviderOptions{TablePrefix: cfg.TablePrefix})
}

// ProvideBundleWithMainDeps wires cache invalidators from the main service layer.
func ProvideBundleWithMainDeps(
	db *sql.DB,
	authCacheInvalidator service.APIKeyAuthCacheInvalidator,
	billingCacheService *service.BillingCacheService,
) (*Bundle, error) {
	bundle, err := ProvideBundleFromEnv(db)
	if err != nil {
		return nil, err
	}
	if bundle.Service != nil {
		bundle.Service.
			WithAuthCacheInvalidator(authCacheInvalidator).
			WithBalanceCacheInvalidator(billingCacheService)
	}
	return bundle, nil
}
