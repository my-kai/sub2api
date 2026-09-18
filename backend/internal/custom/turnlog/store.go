package turnlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Store persists turn logs and the singleton retention policy.
type Store struct{ db *sql.DB }

// NewStore creates a store backed by the application's PostgreSQL connection.
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("turn log database is required")
	}
	return &Store{db: db}, nil
}

func (s *Store) insert(ctx context.Context, event queuedEvent, accountName string) error {
	headers, err := json.Marshal(event.Headers)
	if err != nil {
		return fmt.Errorf("marshal turn log headers: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO custom_turn_logs
			(account_id, account_name, status_code, response_headers, response_body,
			 headers_truncated, body_truncated, body_complete)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)
	`, event.AccountID, accountName, event.StatusCode, headers, event.ResponseBody,
		event.HeadersTruncated, event.BodyTruncated, event.BodyComplete)
	if err != nil {
		return fmt.Errorf("insert turn log: %w", err)
	}
	return nil
}

func (s *Store) list(ctx context.Context, filter TurnLogFilter) (TurnLogPage, error) {
	args := make([]any, 0, 4)
	where := " WHERE TRUE"
	if filter.AccountID != nil {
		args = append(args, *filter.AccountID)
		where += fmt.Sprintf(" AND account_id = $%d", len(args))
	}
	if filter.StatusCode != nil {
		args = append(args, *filter.StatusCode)
		where += fmt.Sprintf(" AND status_code = $%d", len(args))
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM custom_turn_logs"+where, args...).Scan(&total); err != nil {
		return TurnLogPage{}, fmt.Errorf("count turn logs: %w", err)
	}
	args = append(args, filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, account_name, status_code, response_headers,
			response_body, headers_truncated, body_truncated, body_complete, created_at
		FROM custom_turn_logs`+where+fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return TurnLogPage{}, fmt.Errorf("list turn logs: %w", err)
	}
	defer rows.Close()
	items := make([]TurnLog, 0, filter.PageSize)
	for rows.Next() {
		item, err := scanTurnLog(rows)
		if err != nil {
			return TurnLogPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return TurnLogPage{}, fmt.Errorf("iterate turn logs: %w", err)
	}
	return TurnLogPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *Store) get(ctx context.Context, id int64) (TurnLog, error) {
	return scanTurnLog(s.db.QueryRowContext(ctx, `
		SELECT id, account_id, account_name, status_code, response_headers,
			response_body, headers_truncated, body_truncated, body_complete, created_at
		FROM custom_turn_logs WHERE id = $1
	`, id))
}

func (s *Store) loadConfig(ctx context.Context) (TurnLogConfig, error) {
	var config TurnLogConfig
	if err := s.db.QueryRowContext(ctx, `
		SELECT retention_days, updated_at, updated_by
		FROM custom_turn_log_configs WHERE id = 1
	`).Scan(&config.RetentionDays, &config.UpdatedAt, &config.UpdatedBy); err != nil {
		return TurnLogConfig{}, fmt.Errorf("load turn log config: %w", err)
	}
	return config, nil
}

func (s *Store) saveConfig(ctx context.Context, retentionDays int, updatedBy int64) (TurnLogConfig, error) {
	var config TurnLogConfig
	if err := s.db.QueryRowContext(ctx, `
		UPDATE custom_turn_log_configs
		SET retention_days = $1, updated_at = NOW(), updated_by = $2
		WHERE id = 1
		RETURNING retention_days, updated_at, updated_by
	`, retentionDays, updatedBy).Scan(&config.RetentionDays, &config.UpdatedAt, &config.UpdatedBy); err != nil {
		return TurnLogConfig{}, fmt.Errorf("save turn log config: %w", err)
	}
	return config, nil
}

func (s *Store) deleteExpired(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM custom_turn_logs WHERE created_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("delete expired turn logs: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read expired turn log count: %w", err)
	}
	return count, nil
}

type scanner interface{ Scan(...any) error }

func scanTurnLog(row scanner) (TurnLog, error) {
	var item TurnLog
	var headers []byte
	if err := row.Scan(&item.ID, &item.AccountID, &item.AccountName, &item.StatusCode, &headers,
		&item.ResponseBody, &item.HeadersTruncated, &item.BodyTruncated, &item.BodyComplete, &item.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TurnLog{}, sql.ErrNoRows
		}
		return TurnLog{}, fmt.Errorf("scan turn log: %w", err)
	}
	if err := json.Unmarshal(headers, &item.ResponseHeaders); err != nil {
		return TurnLog{}, fmt.Errorf("decode turn log headers: %w", err)
	}
	if item.ResponseHeaders == nil {
		item.ResponseHeaders = map[string][]string{}
	}
	return item, nil
}
