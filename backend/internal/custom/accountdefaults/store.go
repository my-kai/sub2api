package accountdefaults

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Store persists the single-row account-defaults configuration.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// NewStore creates the PostgreSQL-backed account-defaults store.
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("sql db is required")
	}
	return &Store{db: db, now: func() time.Time { return time.Now().UTC() }}, nil
}

// loadConfig reads the singleton row, treating an empty/absent row as zero config.
func (s *Store) loadConfig(ctx context.Context) (Config, error) {
	if s == nil || s.db == nil {
		return Config{}, errors.New("account defaults store is not initialized")
	}
	var raw []byte
	row := s.db.QueryRowContext(ctx, `
		SELECT config FROM custom_account_defaults WHERE id = 1
	`)
	if err := row.Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return normalizeConfig(Config{}), nil
		}
		return Config{}, fmt.Errorf("load account defaults: %w", err)
	}
	var cfg Config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			// 历史脏数据不应让整个配置读取失败：退化为零值配置，由管理员重新保存修复。
			return normalizeConfig(Config{}), nil
		}
	}
	return normalizeConfig(cfg), nil
}

// saveConfig persists the normalized configuration and audit fields.
func (s *Store) saveConfig(ctx context.Context, cfg Config, updatedBy int64) (Config, error) {
	if s == nil || s.db == nil {
		return Config{}, errors.New("account defaults store is not initialized")
	}
	cfg = normalizeConfig(cfg)
	payload, err := json.Marshal(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("marshal account defaults: %w", err)
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE custom_account_defaults
		SET config = $1, updated_at = NOW(), updated_by = $2
		WHERE id = 1
		RETURNING config
	`, payload, updatedBy)
	var saved []byte
	if err := row.Scan(&saved); err != nil {
		return Config{}, fmt.Errorf("save account defaults: %w", err)
	}
	var out Config
	if len(saved) > 0 {
		_ = json.Unmarshal(saved, &out)
	}
	return normalizeConfig(out), nil
}

// normalizeConfig cleans copy semantics (never share backing arrays with callers)
// and drops obviously invalid entries so stored JSON stays predictable.
func normalizeConfig(cfg Config) Config {
	out := Config{
		ModelsByPlatform: map[string][]string{},
		ProxyMode:        cfg.ProxyMode,
		ProxyFixedIDs:    []int64{},
		AllowLocalEgress: cfg.AllowLocalEgress,
	}
	for platform, models := range cfg.ModelsByPlatform {
		key := strings.ToLower(strings.TrimSpace(platform))
		if key == "" {
			continue
		}
		seen := make(map[string]struct{}, len(models))
		clean := make([]string, 0, len(models))
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m == "" {
				continue
			}
			if _, dup := seen[m]; dup {
				continue
			}
			seen[m] = struct{}{}
			clean = append(clean, m)
		}
		out.ModelsByPlatform[key] = clean
	}
	seenID := make(map[int64]struct{}, len(cfg.ProxyFixedIDs))
	for _, id := range cfg.ProxyFixedIDs {
		if id <= 0 {
			continue
		}
		if _, dup := seenID[id]; dup {
			continue
		}
		seenID[id] = struct{}{}
		out.ProxyFixedIDs = append(out.ProxyFixedIDs, id)
	}
	switch out.ProxyMode {
	case ProxyModeFixed, ProxyModeRandom:
	default:
		out.ProxyMode = ""
	}
	return out
}
