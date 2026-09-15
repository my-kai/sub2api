package promptauditv2

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Store owns all prompt-audit-v2 SQL. Keeping raw SQL here avoids adding custom
// tables to the upstream ent schema and generated model surface.
type Store struct {
	db        *sql.DB
	encryptor service.SecretEncryptor
}

// NewStore validates the required persistence and encryption dependencies.
func NewStore(db *sql.DB, encryptor service.SecretEncryptor) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("prompt audit v2 sql db is required")
	}
	if encryptor == nil {
		return nil, fmt.Errorf("prompt audit v2 secret encryptor is required")
	}
	return &Store{db: db, encryptor: encryptor}, nil
}

// LoadConfig returns the persisted configuration including endpoint keys.
// Callers must pass it through PublicConfig before returning it over HTTP.
func (s *Store) LoadConfig(ctx context.Context) (Config, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return Config{}, fmt.Errorf("begin prompt audit v2 config read: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	config, err := loadConfig(ctx, tx)
	if err != nil {
		return Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return Config{}, fmt.Errorf("commit prompt audit v2 config read: %w", err)
	}
	return config, nil
}

// SaveConfig atomically applies a full configuration replacement after a CAS
// check. Existing endpoint keys are preserved only when the same endpoint ID is
// submitted without a replacement key.
func (s *Store) SaveConfig(ctx context.Context, request UpdateConfigRequest, adminID int64) (Config, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Config{}, fmt.Errorf("begin prompt audit v2 config transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := loadConfig(ctx, tx)
	if err != nil {
		return Config{}, err
	}
	if existing.ConfigVersion != request.ExpectedConfigVersion {
		return Config{}, ErrConfigConflict.WithMetadata(map[string]string{
			"current_version": fmt.Sprint(existing.ConfigVersion),
		})
	}
	prepared, err := PrepareConfig(request, &existing)
	if err != nil {
		return Config{}, err
	}
	// Validate the complete runtime snapshot before changing any rows so an
	// invalid enabled endpoint cannot leave a newly submitted configuration persisted.
	if _, err := BuildRuntimeConfig(prepared); err != nil {
		return Config{}, err
	}
	protocolJSON, err := json.Marshal(prepared.EnabledProtocols)
	if err != nil {
		return Config{}, fmt.Errorf("marshal prompt audit v2 protocols: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE custom_prompt_audit_v2_configs
		SET enabled = $1, prompt_template = $2, worker_count = $3,
			queue_capacity = $4, enabled_protocols = $5::jsonb,
			confidence_threshold = $6, window_minutes = $7, trigger_count = $8,
			action = $9, restriction_minutes = $10,
			log_retention_days = $11, config_version = config_version + 1,
			updated_at = NOW(), updated_by = $12
		WHERE id = 1 AND config_version = $13
	`, prepared.Enabled, prepared.PromptTemplate, prepared.WorkerCount, prepared.QueueCapacity,
		string(protocolJSON), prepared.Rule.ConfidenceThreshold, prepared.Rule.WindowMinutes,
		prepared.Rule.TriggerCount, prepared.Rule.Action, prepared.Rule.RestrictionMinutes,
		prepared.LogRetentionDays, nullablePositiveID(adminID), request.ExpectedConfigVersion)
	if err != nil {
		return Config{}, fmt.Errorf("update prompt audit v2 config: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Config{}, fmt.Errorf("read prompt audit v2 config update count: %w", err)
	}
	if affected != 1 {
		return Config{}, ErrConfigConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM custom_prompt_audit_v2_endpoints`); err != nil {
		return Config{}, fmt.Errorf("replace prompt audit v2 endpoints: %w", err)
	}
	for _, endpoint := range prepared.Endpoints {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO custom_prompt_audit_v2_endpoints
				(id, name, base_url, api_key, model, priority, timeout_ms, enabled, config_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, endpoint.ID, endpoint.Name, endpoint.BaseURL, endpoint.APIKey,
			endpoint.Model, endpoint.Priority, endpoint.TimeoutMS, endpoint.Enabled, endpoint.Order); err != nil {
			return Config{}, fmt.Errorf("insert prompt audit v2 endpoint %q: %w", endpoint.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Config{}, fmt.Errorf("commit prompt audit v2 config: %w", err)
	}
	return s.LoadConfig(ctx)
}

// ActiveRestriction reads the database-authoritative restriction. Expired
// warning rows are retained for auditability but are not returned as active.
func (s *Store) ActiveRestriction(ctx context.Context, userID int64, now time.Time) (*Restriction, error) {
	if userID <= 0 {
		return nil, nil
	}
	if now.IsZero() {
		return nil, fmt.Errorf("active restriction check time is required")
	}
	var restriction Restriction
	var eventID sql.NullInt64
	var blockedUntil sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, event_id, action, started_at, blocked_until
		FROM custom_prompt_audit_v2_restrictions AS restriction
		JOIN users AS account_user ON account_user.id = restriction.user_id
		WHERE restriction.user_id = $1
		  AND ((restriction.action = 'ban' AND account_user.status = 'disabled') OR restriction.blocked_until > $2)
	`, userID, now).Scan(&restriction.UserID, &eventID,
		&restriction.Action, &restriction.StartedAt, &blockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load prompt audit v2 restriction: %w", err)
	}
	if blockedUntil.Valid {
		restriction.BlockedUntil = &blockedUntil.Time
	}
	if eventID.Valid {
		restriction.EventID = &eventID.Int64
	}
	return &restriction, nil
}

// ListEvents returns only rule-hit events. The complete message ciphertext is
// deliberately omitted from the list query and response.
func (s *Store) ListEvents(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error) {
	where, args := buildEventWhere(filter)
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM custom_prompt_audit_v2_events`+where, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count prompt audit v2 events: %w", err)
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, eventListSelect+where+fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("list prompt audit v2 events: %w", err)
	}
	defer rows.Close()
	items := make([]Event, 0, pageSize)
	for rows.Next() {
		event, err := scanEvent(rows, false)
		if err != nil {
			return nil, fmt.Errorf("scan prompt audit v2 event: %w", err)
		}
		items = append(items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prompt audit v2 events: %w", err)
	}
	return &EventPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

// GetEvent returns one hit including ciphertext for a later explicit decrypt.
func (s *Store) GetEvent(ctx context.Context, id int64) (*Event, error) {
	event, err := scanEvent(s.db.QueryRowContext(ctx, eventDetailSelect+` WHERE id = $1`, id), true)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEventNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get prompt audit v2 event: %w", err)
	}
	return &event, nil
}

type sqlQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type rowScanner interface {
	Scan(...any) error
}

func loadConfig(ctx context.Context, queryer sqlQueryer) (Config, error) {
	var config Config
	var protocolsJSON []byte
	var updatedBy sql.NullInt64
	var restrictionMinutes sql.NullInt64
	err := queryer.QueryRowContext(ctx, `
		SELECT enabled, prompt_template, worker_count, queue_capacity,
			enabled_protocols, confidence_threshold, window_minutes, trigger_count,
			action, restriction_minutes, log_retention_days, config_version, updated_at, updated_by
		FROM custom_prompt_audit_v2_configs WHERE id = 1
	`).Scan(&config.Enabled, &config.PromptTemplate, &config.WorkerCount, &config.QueueCapacity,
		&protocolsJSON, &config.Rule.ConfidenceThreshold, &config.Rule.WindowMinutes,
		&config.Rule.TriggerCount, &config.Rule.Action, &restrictionMinutes,
		&config.LogRetentionDays, &config.ConfigVersion, &config.UpdatedAt, &updatedBy)
	if err != nil {
		return Config{}, fmt.Errorf("load prompt audit v2 config: %w", err)
	}
	if err := json.Unmarshal(protocolsJSON, &config.EnabledProtocols); err != nil {
		return Config{}, fmt.Errorf("decode prompt audit v2 protocols: %w", err)
	}
	if restrictionMinutes.Valid {
		value := int(restrictionMinutes.Int64)
		config.Rule.RestrictionMinutes = &value
	}
	if updatedBy.Valid {
		config.UpdatedBy = &updatedBy.Int64
	}
	endpointRows, err := queryer.QueryContext(ctx, `
		SELECT id, name, base_url, api_key, model, priority, timeout_ms, enabled, config_order
		FROM custom_prompt_audit_v2_endpoints ORDER BY config_order, id
	`)
	if err != nil {
		return Config{}, fmt.Errorf("load prompt audit v2 endpoints: %w", err)
	}
	for endpointRows.Next() {
		var endpoint EndpointConfig
		if err := endpointRows.Scan(&endpoint.ID, &endpoint.Name, &endpoint.BaseURL, &endpoint.APIKey,
			&endpoint.Model, &endpoint.Priority, &endpoint.TimeoutMS, &endpoint.Enabled, &endpoint.Order); err != nil {
			_ = endpointRows.Close()
			return Config{}, fmt.Errorf("scan prompt audit v2 endpoint: %w", err)
		}
		endpoint.HasAPIKey = strings.TrimSpace(endpoint.APIKey) != ""
		config.Endpoints = append(config.Endpoints, endpoint)
	}
	if err := endpointRows.Close(); err != nil {
		return Config{}, fmt.Errorf("close prompt audit v2 endpoint rows: %w", err)
	}
	if err := endpointRows.Err(); err != nil {
		return Config{}, fmt.Errorf("iterate prompt audit v2 endpoints: %w", err)
	}
	return config, nil
}

const eventColumns = `
	id, request_id, user_id, username_snapshot, user_email_snapshot,
	api_key_id, api_key_name_snapshot, protocol, request_model,
	endpoint_id, endpoint_name_snapshot, audit_model, confidence, reason, latency_ms,
	rule_threshold, rule_window_minutes, rule_trigger_count,
	rule_action, rule_restriction_minutes, window_hit_count, threshold_reached,
	final_action, action_result, message_sha256, message_chars, created_at`

const eventListSelect = `SELECT ` + eventColumns + ` FROM custom_prompt_audit_v2_events`
const eventDetailSelect = `SELECT ` + eventColumns + `, message_ciphertext FROM custom_prompt_audit_v2_events`

func scanEvent(row rowScanner, includeCiphertext bool) (Event, error) {
	var event Event
	var restrictionMinutes sql.NullInt64
	values := []any{
		&event.ID, &event.RequestID, &event.UserID, &event.Username, &event.UserEmail,
		&event.APIKeyID, &event.APIKeyName, &event.Protocol, &event.RequestModel,
		&event.EndpointID, &event.EndpointName, &event.AuditModel, &event.Confidence,
		&event.Reason, &event.LatencyMS, &event.RuleThreshold,
		&event.RuleWindowMinutes, &event.RuleTriggerCount, &event.RuleAction, &restrictionMinutes,
		&event.WindowHitCount, &event.ThresholdReached, &event.FinalAction, &event.ActionResult,
		&event.MessageSHA256, &event.MessageChars, &event.CreatedAt,
	}
	if includeCiphertext {
		values = append(values, &event.MessageCiphertext)
	}
	if err := row.Scan(values...); err != nil {
		return Event{}, err
	}
	if restrictionMinutes.Valid {
		value := int(restrictionMinutes.Int64)
		event.RuleRestrictionMinutes = &value
	}
	return event, nil
}

func buildEventWhere(filter EventFilter) (string, []any) {
	conditions := make([]string, 0, 8)
	args := make([]any, 0, 8)
	add := func(column, operator string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("%s %s $%d", column, operator, len(args)))
	}
	if filter.StartAt != nil {
		add("created_at", ">=", *filter.StartAt)
	}
	if filter.EndAt != nil {
		add("created_at", "<=", *filter.EndAt)
	}
	if filter.UserID != nil {
		add("user_id", "=", *filter.UserID)
	}
	if filter.Action != "" {
		add("final_action", "=", filter.Action)
	}
	if filter.Protocol != "" {
		add("protocol", "=", filter.Protocol)
	}
	if filter.MinConfidence != nil {
		add("confidence", ">=", *filter.MinConfidence)
	}
	if filter.MaxConfidence != nil {
		add("confidence", "<=", *filter.MaxConfidence)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

func nullablePositiveID(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
