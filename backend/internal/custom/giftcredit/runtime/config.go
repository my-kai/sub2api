package runtime

import (
	"os"
	"strings"
	"time"
)

const defaultMigrationTimeout = 30 * time.Second

// Config stores custom gift-credit startup options outside upstream config.
type Config struct {
	TablePrefix      string
	MigrationTimeout time.Duration
}

// LoadConfigFromEnv reads optional gift-credit runtime configuration.
//
// Supported variables:
//   - CUSTOM_GIFT_CREDIT_TABLE_PREFIX: optional prefix for all gift-credit-owned tables.
//   - CUSTOM_GIFT_CREDIT_MIGRATION_TIMEOUT: timeout for startup migration execution.
func LoadConfigFromEnv() (Config, error) {
	timeout, err := parseDurationWithDefault(os.Getenv("CUSTOM_GIFT_CREDIT_MIGRATION_TIMEOUT"), defaultMigrationTimeout)
	if err != nil {
		return Config{}, err
	}
	return Config{
		TablePrefix:      strings.TrimSpace(os.Getenv("CUSTOM_GIFT_CREDIT_TABLE_PREFIX")),
		MigrationTimeout: timeout,
	}, nil
}

func parseDurationWithDefault(raw string, fallback time.Duration) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, ErrInvalidConfig("CUSTOM_GIFT_CREDIT_MIGRATION_TIMEOUT must be positive")
	}
	return parsed, nil
}

// ErrInvalidConfig marks a custom gift-credit startup configuration error.
type ErrInvalidConfig string

func (e ErrInvalidConfig) Error() string {
	return string(e)
}
