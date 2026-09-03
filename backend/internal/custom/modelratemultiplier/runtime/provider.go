package runtime

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/custom/modelratemultiplier/service"
	"github.com/Wei-Shaw/sub2api/internal/custom/modelratemultiplier/store"
	modelratemigrations "github.com/Wei-Shaw/sub2api/migrations/custom/modelratemultiplier"
)

// Bundle contains the custom model-rate runtime dependencies.
type Bundle struct {
	Store   *store.Store
	Service *service.Service
}

// ProvideBundle applies migrations and constructs the model-rate service.
func ProvideBundle(db *sql.DB, userRate service.UserRateReader) (*Bundle, error) {
	if db == nil {
		return nil, fmt.Errorf("sql db is required")
	}
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.MigrationTimeout)
	defer cancel()
	if err := ApplyMigrationsFS(ctx, db, modelratemigrations.FS, cfg.TablePrefix, defaultMigrationsTable); err != nil {
		return nil, err
	}
	modelStore, err := store.NewStore(db, cfg.TablePrefix)
	if err != nil {
		return nil, err
	}
	return &Bundle{Store: modelStore, Service: service.NewService(modelStore, userRate)}, nil
}
