package runtime

import (
	"os"
	"strings"
	"time"
)

const defaultMigrationTimeout = 30 * time.Second

// Config stores custom activity startup options that should not be added to
// the upstream global config structure.
type Config struct {
	TablePrefix      string
	MigrationTimeout time.Duration
}

// LoadConfigFromEnv reads optional custom activity runtime configuration.
//
// Supported variables:
//   - CUSTOM_ACTIVITY_TABLE_PREFIX: optional prefix for all activity-owned tables.
//   - CUSTOM_ACTIVITY_MIGRATION_TIMEOUT: timeout for startup migration execution.
func LoadConfigFromEnv() (Config, error) {
	timeout, err := parseDurationWithDefault(os.Getenv("CUSTOM_ACTIVITY_MIGRATION_TIMEOUT"), defaultMigrationTimeout)
	if err != nil {
		return Config{}, err
	}
	return Config{
		TablePrefix:      strings.TrimSpace(os.Getenv("CUSTOM_ACTIVITY_TABLE_PREFIX")),
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
		return 0, ErrInvalidConfig("CUSTOM_ACTIVITY_MIGRATION_TIMEOUT must be positive")
	}
	return parsed, nil
}

// ErrInvalidConfig marks a custom activity startup configuration error.
type ErrInvalidConfig string

func (e ErrInvalidConfig) Error() string {
	return string(e)
}

func validateIdentifierPart(value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return ErrInvalidConfig("identifier is required")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return ErrInvalidConfig("identifier may only contain letters, digits or underscores")
	}
	return nil
}
