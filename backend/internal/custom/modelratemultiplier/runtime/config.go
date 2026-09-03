package runtime

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// defaultMigrationTimeout bounds startup migration work when no environment override is set.
const defaultMigrationTimeout = 30 * time.Second

// Config controls optional model-rate custom runtime settings.
type Config struct {
	TablePrefix      string
	MigrationTimeout time.Duration
}

// LoadConfigFromEnv reads model-rate migration settings.
func LoadConfigFromEnv() (Config, error) {
	timeout := defaultMigrationTimeout
	if raw := strings.TrimSpace(os.Getenv("CUSTOM_MODEL_RATE_MIGRATION_TIMEOUT")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, err
		}
		if parsed <= 0 {
			return Config{}, fmt.Errorf("CUSTOM_MODEL_RATE_MIGRATION_TIMEOUT must be positive")
		}
		timeout = parsed
	}
	return Config{TablePrefix: strings.TrimSpace(os.Getenv("CUSTOM_MODEL_RATE_TABLE_PREFIX")), MigrationTimeout: timeout}, nil
}
