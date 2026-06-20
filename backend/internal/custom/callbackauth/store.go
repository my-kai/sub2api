package callbackauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Store owns callback authorization and one-time code persistence.
type Store struct {
	db  *sql.DB
	now func() time.Time
	ttl time.Duration
}

// NewStore creates a PostgreSQL-backed callback authorization store.
func NewStore(db *sql.DB, ttl time.Duration) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("sql db is required")
	}
	return &Store{
		db:  db,
		now: time.Now,
		ttl: normalizeTTL(ttl),
	}, nil
}

// EnsureSchema creates the custom tables if they are missing. The schema is
// intentionally isolated from the main migration sequence to reduce upstream
// merge conflicts for this custom login handoff feature.
func (s *Store) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("callback auth store is not initialized")
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS custom_callback_authorizations (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			callback_domain TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (user_id, callback_domain)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_callback_authorizations_user_id
			ON custom_callback_authorizations(user_id)`,
		`CREATE TABLE IF NOT EXISTS custom_callback_auth_codes (
			code_hash TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			callback_url TEXT NOT NULL,
			callback_domain TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			consumed_at TIMESTAMPTZ NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_callback_auth_codes_expires_at
			ON custom_callback_auth_codes(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_callback_auth_codes_user_id
			ON custom_callback_auth_codes(user_id)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure callback auth schema: %w", err)
		}
	}
	return nil
}

// IsAuthorized checks whether the user has previously confirmed this callback domain.
func (s *Store) IsAuthorized(ctx context.Context, userID int64, domain string) (bool, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if s == nil || s.db == nil || userID <= 0 || domain == "" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM custom_callback_authorizations
			WHERE user_id = $1 AND callback_domain = $2
		)
	`, userID, domain).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("query callback authorization: %w", err)
	}
	return exists, nil
}

// UpsertAuthorization records durable user consent for a callback domain.
func (s *Store) UpsertAuthorization(ctx context.Context, userID int64, domain string) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if s == nil || s.db == nil || userID <= 0 || domain == "" {
		return fmt.Errorf("invalid callback authorization")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO custom_callback_authorizations (user_id, callback_domain, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		ON CONFLICT (user_id, callback_domain)
		DO UPDATE SET updated_at = NOW()
	`, userID, domain)
	if err != nil {
		return fmt.Errorf("upsert callback authorization: %w", err)
	}
	return nil
}

// CreateCode stores a hashed one-time code and returns the plaintext only once
// so database reads cannot replay login handoff codes.
func (s *Store) CreateCode(ctx context.Context, userID int64, callbackURL, domain string) (string, codeRecord, error) {
	callbackURL = strings.TrimSpace(callbackURL)
	domain = strings.TrimSpace(strings.ToLower(domain))
	if s == nil || s.db == nil || userID <= 0 || callbackURL == "" || domain == "" {
		return "", codeRecord{}, fmt.Errorf("invalid callback auth code")
	}

	code, err := randomCode()
	if err != nil {
		return "", codeRecord{}, err
	}
	now := s.now().UTC()
	record := codeRecord{
		UserID:         userID,
		CallbackURL:    callbackURL,
		CallbackDomain: domain,
		ExpiresAt:      now.Add(s.ttl).UTC(),
		CreatedAt:      now,
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO custom_callback_auth_codes (
			code_hash, user_id, callback_url, callback_domain, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, hashCode(code), userID, callbackURL, domain, record.ExpiresAt, record.CreatedAt)
	if err != nil {
		return "", codeRecord{}, fmt.Errorf("create callback auth code: %w", err)
	}
	return code, record, nil
}

// ConsumeCode atomically marks a code as consumed and returns its handoff target.
func (s *Store) ConsumeCode(ctx context.Context, code string) (codeRecord, error) {
	code = strings.TrimSpace(code)
	if s == nil || s.db == nil || code == "" {
		return codeRecord{}, ErrCodeExpired
	}
	now := s.now().UTC()
	row := s.db.QueryRowContext(ctx, `
		UPDATE custom_callback_auth_codes
		SET consumed_at = $2
		WHERE code_hash = $1
		  AND consumed_at IS NULL
		  AND expires_at > $2
		RETURNING user_id, callback_url, callback_domain, expires_at, created_at
	`, hashCode(code), now)

	var record codeRecord
	if err := row.Scan(&record.UserID, &record.CallbackURL, &record.CallbackDomain, &record.ExpiresAt, &record.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return codeRecord{}, ErrCodeExpired
		}
		return codeRecord{}, fmt.Errorf("consume callback auth code: %w", err)
	}
	record.ExpiresAt = record.ExpiresAt.UTC()
	record.CreatedAt = record.CreatedAt.UTC()
	return record, nil
}

func normalizeTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultCodeTTL
	}
	if ttl < minCodeTTL {
		return minCodeTTL
	}
	if ttl > maxCodeTTL {
		return maxCodeTTL
	}
	return ttl
}

func randomCode() (string, error) {
	raw := make([]byte, codeByteLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return codePrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}
