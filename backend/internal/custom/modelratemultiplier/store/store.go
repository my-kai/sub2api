package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/custom/modelratemultiplier/types"
)

// Store owns SQL persistence for group model-rate overrides.
type Store struct {
	db          *sql.DB
	tablePrefix string
}

// NewStore creates a model-rate store using the already-open application DB.
func NewStore(db *sql.DB, tablePrefix string) (*Store, error) {
	if db == nil {
		return nil, errors.New("model rate multiplier store requires sql db")
	}
	if err := validateIdentifierPart(tablePrefix); err != nil {
		return nil, fmt.Errorf("model rate multiplier table prefix is invalid: %w", err)
	}
	return &Store{db: db, tablePrefix: strings.TrimSpace(tablePrefix)}, nil
}

func (s *Store) tableName() string { return s.tablePrefix + "group_model_rate_multipliers" }

// ListByGroupID returns entries in stable normalized-model order.
func (s *Store) ListByGroupID(ctx context.Context, groupID int64) ([]types.Entry, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("model rate multiplier store is not configured")
	}
	if groupID <= 0 {
		return nil, errors.New("group id must be positive")
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, group_id, model, rate_multiplier, created_at, updated_at FROM "+s.tableName()+" WHERE group_id = $1 ORDER BY lower(model), model", groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	entries := make([]types.Entry, 0)
	for rows.Next() {
		var entry types.Entry
		if err := rows.Scan(&entry.ID, &entry.GroupID, &entry.Model, &entry.RateMultiplier, &entry.CreatedAt, &entry.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// ReplaceByGroupID atomically replaces all overrides for one group.
func (s *Store) ReplaceByGroupID(ctx context.Context, groupID int64, entries []types.Input) error {
	if s == nil || s.db == nil {
		return errors.New("model rate multiplier store is not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin model rate multiplier replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	table := s.tableName()
	if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE group_id = $1", groupID); err != nil {
		return fmt.Errorf("clear model rate multipliers: %w", err)
	}
	for _, entry := range entries {
		if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+" (group_id, model, rate_multiplier) VALUES ($1, $2, $3)", groupID, entry.Model, entry.RateMultiplier); err != nil {
			return fmt.Errorf("insert model rate multiplier %q: %w", entry.Model, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model rate multiplier replacement: %w", err)
	}
	return nil
}

// ClearByGroupID removes all overrides for a group.
func (s *Store) ClearByGroupID(ctx context.Context, groupID int64) error {
	if s == nil || s.db == nil {
		return errors.New("model rate multiplier store is not configured")
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM "+s.tableName()+" WHERE group_id = $1", groupID)
	return err
}

func validateIdentifierPart(value string) error {
	for _, r := range strings.TrimSpace(value) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return errors.New("identifier may only contain letters, digits or underscores")
	}
	return nil
}
