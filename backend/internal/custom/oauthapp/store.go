package oauthapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Store 负责 OAuth 应用和授权码的持久化。
type Store struct {
	db  *sql.DB
	now func() time.Time
	ttl time.Duration
}

// NewStore 创建基于 PostgreSQL 的 OAuth 应用存储。
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

// EnsureSchema 在上游迁移序列之外创建自定义 OAuth 应用表。
// 这样后续合并上游迁移时，不会和二开表结构编号发生冲突。
func (s *Store) EnsureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("oauth application store is not initialized")
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS custom_oauth_applications (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			access_key TEXT NOT NULL UNIQUE,
			access_secret_hash TEXT NOT NULL,
			allowed_domains JSONB NOT NULL DEFAULT '[]'::jsonb,
			status TEXT NOT NULL DEFAULT 'enabled',
			deleted_at TIMESTAMPTZ NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_oauth_applications_access_key
			ON custom_oauth_applications(access_key)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_oauth_applications_deleted_at
			ON custom_oauth_applications(deleted_at)`,
		`CREATE TABLE IF NOT EXISTS custom_oauth_authorization_codes (
			code_hash TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			access_key TEXT NOT NULL,
			redirect_uri TEXT NOT NULL,
			redirect_domain TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			consumed_at TIMESTAMPTZ NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_oauth_authorization_codes_expires_at
			ON custom_oauth_authorization_codes(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_custom_oauth_authorization_codes_access_key
			ON custom_oauth_authorization_codes(access_key)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure oauth application schema: %w", err)
		}
	}
	return nil
}

// ListApplications 按创建时间倒序返回未删除应用。
func (s *Store) ListApplications(ctx context.Context) ([]Application, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("oauth application store is not initialized")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, access_key, access_secret_hash, allowed_domains, status, created_at, updated_at, deleted_at
		FROM custom_oauth_applications
		WHERE deleted_at IS NULL
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list oauth applications: %w", err)
	}
	defer rows.Close()

	apps := []Application{}
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate oauth applications: %w", err)
	}
	return apps, nil
}

// GetApplication 通过数字 ID 读取单个未删除应用。
func (s *Store) GetApplication(ctx context.Context, id int64) (*Application, error) {
	if s == nil || s.db == nil || id <= 0 {
		return nil, ErrInvalidApplication
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, access_key, access_secret_hash, allowed_domains, status, created_at, updated_at, deleted_at
		FROM custom_oauth_applications
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidApplication
		}
		return nil, err
	}
	return &app, nil
}

// GetApplicationByAccessKey 通过公开客户端 ID 读取未删除应用。
func (s *Store) GetApplicationByAccessKey(ctx context.Context, accessKey string) (*Application, error) {
	accessKey = strings.TrimSpace(accessKey)
	if s == nil || s.db == nil || accessKey == "" {
		return nil, ErrInvalidApplication
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, access_key, access_secret_hash, allowed_domains, status, created_at, updated_at, deleted_at
		FROM custom_oauth_applications
		WHERE access_key = $1 AND deleted_at IS NULL
	`, accessKey)
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidApplication
		}
		return nil, err
	}
	return &app, nil
}

// CreateApplication 创建应用，并生成一组新的客户端密钥。
func (s *Store) CreateApplication(ctx context.Context, name string, domains []string, status ApplicationStatus) (*Application, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", ErrInvalidApplication
	}
	normalizedDomains, err := NormalizeAllowedDomains(domains)
	if err != nil {
		return nil, "", err
	}
	accessKey, err := randomAccessKey()
	if err != nil {
		return nil, "", err
	}
	secret, err := randomSecret()
	if err != nil {
		return nil, "", err
	}
	secretHash, err := hashSecret(secret)
	if err != nil {
		return nil, "", err
	}
	domainJSON, err := json.Marshal(normalizedDomains)
	if err != nil {
		return nil, "", err
	}
	if status == "" {
		status = ApplicationStatusEnabled
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO custom_oauth_applications (
			name, access_key, access_secret_hash, allowed_domains, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4::jsonb, $5, NOW(), NOW())
		RETURNING id, name, access_key, access_secret_hash, allowed_domains, status, created_at, updated_at, deleted_at
	`, name, accessKey, secretHash, string(domainJSON), string(status))
	app, err := scanApplication(row)
	if err != nil {
		return nil, "", err
	}
	return &app, secret, nil
}

// UpdateApplication 替换应用可变字段，同时保留原客户端密钥。
func (s *Store) UpdateApplication(ctx context.Context, id int64, name string, domains []string, status ApplicationStatus) (*Application, error) {
	name = strings.TrimSpace(name)
	if s == nil || s.db == nil || id <= 0 || name == "" {
		return nil, ErrInvalidApplication
	}
	normalizedDomains, err := NormalizeAllowedDomains(domains)
	if err != nil {
		return nil, err
	}
	domainJSON, err := json.Marshal(normalizedDomains)
	if err != nil {
		return nil, err
	}
	if status == "" {
		status = ApplicationStatusEnabled
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE custom_oauth_applications
		SET name = $2, allowed_domains = $3::jsonb, status = $4, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, name, access_key, access_secret_hash, allowed_domains, status, created_at, updated_at, deleted_at
	`, id, name, string(domainJSON), string(status))
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidApplication
		}
		return nil, err
	}
	return &app, nil
}

// ResetSecret 轮换应用密钥，并只返回一次明文。
func (s *Store) ResetSecret(ctx context.Context, id int64) (*Application, string, error) {
	if s == nil || s.db == nil || id <= 0 {
		return nil, "", ErrInvalidApplication
	}
	secret, err := randomSecret()
	if err != nil {
		return nil, "", err
	}
	secretHash, err := hashSecret(secret)
	if err != nil {
		return nil, "", err
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE custom_oauth_applications
		SET access_secret_hash = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, name, access_key, access_secret_hash, allowed_domains, status, created_at, updated_at, deleted_at
	`, id, secretHash)
	app, err := scanApplication(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", ErrInvalidApplication
		}
		return nil, "", err
	}
	return &app, secret, nil
}

// DeleteApplication 软删除应用，保留既有审计数据可查。
func (s *Store) DeleteApplication(ctx context.Context, id int64) error {
	if s == nil || s.db == nil || id <= 0 {
		return ErrInvalidApplication
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE custom_oauth_applications
		SET deleted_at = NOW(), status = $2, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id, string(ApplicationStatusDisabled))
	if err != nil {
		return fmt.Errorf("delete oauth application: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrInvalidApplication
	}
	return nil
}

// CreateCode 创建并存储 hash 后的一次性授权码。
func (s *Store) CreateCode(ctx context.Context, userID int64, accessKey, redirectURI, redirectDomain string) (string, codeRecord, error) {
	code, err := randomCode()
	if err != nil {
		return "", codeRecord{}, err
	}
	record, err := s.CreateCodeWithValue(ctx, code, userID, accessKey, redirectURI, redirectDomain)
	if err != nil {
		return "", codeRecord{}, err
	}
	return code, record, nil
}

// CreateCodeWithValue 持久化 OAuth 库生成的授权码。
func (s *Store) CreateCodeWithValue(ctx context.Context, code string, userID int64, accessKey, redirectURI, redirectDomain string) (codeRecord, error) {
	code = strings.TrimSpace(code)
	accessKey = strings.TrimSpace(accessKey)
	redirectURI = strings.TrimSpace(redirectURI)
	redirectDomain = strings.TrimSpace(strings.ToLower(redirectDomain))
	if s == nil || s.db == nil || code == "" || userID <= 0 || accessKey == "" || redirectURI == "" || redirectDomain == "" {
		return codeRecord{}, ErrInvalidApplication
	}
	now := s.now().UTC()
	record := codeRecord{
		UserID:         userID,
		AccessKey:      accessKey,
		RedirectURI:    redirectURI,
		RedirectDomain: redirectDomain,
		ExpiresAt:      now.Add(s.ttl).UTC(),
		CreatedAt:      now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO custom_oauth_authorization_codes (
			code_hash, user_id, access_key, redirect_uri, redirect_domain, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, hashCode(code), userID, accessKey, redirectURI, redirectDomain, record.ExpiresAt, record.CreatedAt)
	if err != nil {
		return codeRecord{}, fmt.Errorf("create oauth authorization code: %w", err)
	}
	return record, nil
}

// ConsumeCode 原子消费授权码，避免同一个 code 被重复换取 token。
func (s *Store) ConsumeCode(ctx context.Context, code string) (codeRecord, error) {
	code = strings.TrimSpace(code)
	if s == nil || s.db == nil || code == "" {
		return codeRecord{}, ErrCodeExpired
	}
	now := s.now().UTC()
	row := s.db.QueryRowContext(ctx, `
		UPDATE custom_oauth_authorization_codes
		SET consumed_at = $2
		WHERE code_hash = $1
		  AND consumed_at IS NULL
		  AND expires_at > $2
		RETURNING user_id, access_key, redirect_uri, redirect_domain, expires_at, created_at
	`, hashCode(code), now)

	var record codeRecord
	if err := row.Scan(&record.UserID, &record.AccessKey, &record.RedirectURI, &record.RedirectDomain, &record.ExpiresAt, &record.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return codeRecord{}, ErrCodeExpired
		}
		return codeRecord{}, fmt.Errorf("consume oauth authorization code: %w", err)
	}
	record.ExpiresAt = record.ExpiresAt.UTC()
	record.CreatedAt = record.CreatedAt.UTC()
	return record, nil
}

func scanApplication(scanner interface {
	Scan(dest ...any) error
}) (Application, error) {
	var app Application
	var allowedRaw []byte
	var status string
	var deletedAt sql.NullTime
	if err := scanner.Scan(
		&app.ID,
		&app.Name,
		&app.AccessKey,
		&app.AccessSecretHash,
		&allowedRaw,
		&status,
		&app.CreatedAt,
		&app.UpdatedAt,
		&deletedAt,
	); err != nil {
		return Application{}, err
	}
	if len(allowedRaw) > 0 {
		_ = json.Unmarshal(allowedRaw, &app.AllowedDomains)
	}
	app.Status = normalizeStatus(status, ApplicationStatusDisabled)
	app.CreatedAt = app.CreatedAt.UTC()
	app.UpdatedAt = app.UpdatedAt.UTC()
	if deletedAt.Valid {
		t := deletedAt.Time.UTC()
		app.DeletedAt = &t
	}
	return app, nil
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
