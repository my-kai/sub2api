package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
	"github.com/lib/pq"
)

type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store owns custom activity SQL persistence.
//
// Schema creation is handled by runtime migrations. Keeping Store free of
// auto-DDL prevents request paths from hiding migration drift.
type Store struct {
	db          *sql.DB
	tablePrefix string
}

// NewStore creates a PostgreSQL-backed activity store after migrations have run.
func NewStore(db *sql.DB, tablePrefix string) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("sql db is required")
	}
	if err := validateTablePrefix(tablePrefix); err != nil {
		return nil, err
	}
	return &Store{db: db, tablePrefix: tablePrefix}, nil
}

// DB exposes the underlying SQL connection for thin runtime self-checks.
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// Close keeps the same lifecycle shape as other custom stores; Store does not own db.
func (s *Store) Close() error {
	return nil
}

// CreateActivity inserts a common activity card record.
func (s *Store) CreateActivity(ctx context.Context, input types.Activity) (types.Activity, error) {
	now := normalizeNow(input.CreatedAt)
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO `+s.table("custom_activities")+` (
			type, title, description, cover_url, status, starts_at, ends_at, created_by, created_at, updated_at, ended_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10)
		RETURNING `+activityColumns()+`
	`, input.Type, strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), strings.TrimSpace(input.CoverURL),
		input.Status, input.StartsAt.UTC(), input.EndsAt.UTC(), nullInt64Param(input.CreatedBy), now,
		nullTimeParam(input.EndedAt))
	activity, err := scanActivity(row)
	if err != nil {
		return types.Activity{}, fmt.Errorf("create custom activity: %w", err)
	}
	return activity, nil
}

// GetActivity returns one activity by id.
func (s *Store) GetActivity(ctx context.Context, id int64) (types.Activity, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+activityColumns()+`
		FROM `+s.table("custom_activities")+`
		WHERE id = $1
	`, id)
	activity, err := scanActivity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Activity{}, types.ErrNotFound
	}
	if err != nil {
		return types.Activity{}, fmt.Errorf("query custom activity: %w", err)
	}
	return activity, nil
}

// ListActivities returns activity cards in reverse creation order.
func (s *Store) ListActivities(ctx context.Context, statuses []types.ActivityStatus, page types.PageRequest) ([]types.Activity, int64, error) {
	page = normalizePage(page)
	args := []any{}
	where := ""
	if len(statuses) > 0 {
		statusValues := make([]string, 0, len(statuses))
		for _, status := range statuses {
			statusValues = append(statusValues, string(status))
		}
		args = append(args, pq.Array(statusValues))
		where = "WHERE status = ANY($1)"
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.table("custom_activities")+` `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count custom activities: %w", err)
	}

	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	args = append(args, page.PageSize, (page.Page-1)*page.PageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+activityColumns()+`
		FROM `+s.table("custom_activities")+`
		`+where+`
		ORDER BY created_at DESC, id DESC
		LIMIT $`+fmt.Sprint(limitArg)+` OFFSET $`+fmt.Sprint(offsetArg)+`
	`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list custom activities: %w", err)
	}
	defer rows.Close()
	items, err := scanActivityRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListAdminSummaries returns activity cards with budget and participant aggregates.
//
// The aggregate stays in Store because later handlers need the same issued
// amount and participant count for both list rendering and admin detail checks.
func (s *Store) ListAdminSummaries(ctx context.Context, page types.PageRequest) ([]types.ActivityAdminSummary, int64, error) {
	page = normalizePage(page)
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.table("custom_activities")).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count custom activity admin summaries: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+prefixedActivityColumns("a")+`,
		       COALESCE(cfg.total_budget, 0)::text,
		       COALESCE(SUM(c.reward_amount), 0)::text,
		       COUNT(DISTINCT c.user_id)
		FROM `+s.table("custom_activities")+` a
		LEFT JOIN `+s.table("custom_red_packet_rain_configs")+` cfg ON cfg.activity_id = a.id
		LEFT JOIN `+s.table("custom_red_packet_rain_claims")+` c ON c.activity_id = a.id
		GROUP BY a.id, cfg.total_budget
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $1 OFFSET $2
	`, page.PageSize, (page.Page-1)*page.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list custom activity admin summaries: %w", err)
	}
	defer rows.Close()

	items := []types.ActivityAdminSummary{}
	for rows.Next() {
		item, err := scanAdminSummary(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// UpdateActivity updates common fields for an activity that service code has already allowed.
func (s *Store) UpdateActivity(ctx context.Context, input types.Activity) (types.Activity, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE `+s.table("custom_activities")+`
		SET title = $2,
		    description = $3,
		    cover_url = $4,
		    status = $5,
		    starts_at = $6,
		    ends_at = $7,
		    ended_at = $8,
		    updated_at = $9
		WHERE id = $1
		RETURNING `+activityColumns()+`
	`, input.ID, strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), strings.TrimSpace(input.CoverURL),
		input.Status, input.StartsAt.UTC(), input.EndsAt.UTC(), nullTimeParam(input.EndedAt), normalizeNow(input.UpdatedAt))
	activity, err := scanActivity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Activity{}, types.ErrNotFound
	}
	if err != nil {
		return types.Activity{}, fmt.Errorf("update custom activity: %w", err)
	}
	return activity, nil
}

// SetActivityStatus changes only the lifecycle state for early end/offline flows.
func (s *Store) SetActivityStatus(ctx context.Context, id int64, status types.ActivityStatus, endedAt *time.Time, updatedAt time.Time) (types.Activity, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE `+s.table("custom_activities")+`
		SET status = $2,
		    ended_at = $3,
		    updated_at = $4
		WHERE id = $1
		RETURNING `+activityColumns()+`
	`, id, status, nullTimeParam(endedAt), normalizeNow(updatedAt))
	activity, err := scanActivity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Activity{}, types.ErrNotFound
	}
	if err != nil {
		return types.Activity{}, fmt.Errorf("set custom activity status: %w", err)
	}
	return activity, nil
}

// UpsertRedPacketRainConfig stores admin money and round rules for one activity.
func (s *Store) UpsertRedPacketRainConfig(ctx context.Context, cfg types.RedPacketRainConfig) (types.RedPacketRainConfig, error) {
	now := normalizeNow(cfg.UpdatedAt)
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO `+s.table("custom_red_packet_rain_configs")+` (
			activity_id, round_count, round_duration_seconds, round_interval_seconds,
			total_budget, per_user_round_cap, per_user_total_cap,
			base_unit_amount, max_single_reward, probability_step, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5::decimal, $6::decimal, $7::decimal, $8::decimal, $9::decimal, $10::decimal, $11, $11
		)
		ON CONFLICT (activity_id) DO UPDATE SET
			round_count = EXCLUDED.round_count,
			round_duration_seconds = EXCLUDED.round_duration_seconds,
			round_interval_seconds = EXCLUDED.round_interval_seconds,
			total_budget = EXCLUDED.total_budget,
			per_user_round_cap = EXCLUDED.per_user_round_cap,
			per_user_total_cap = EXCLUDED.per_user_total_cap,
			base_unit_amount = EXCLUDED.base_unit_amount,
			max_single_reward = EXCLUDED.max_single_reward,
			probability_step = EXCLUDED.probability_step,
			updated_at = EXCLUDED.updated_at
		RETURNING `+configColumns()+`
	`, cfg.ActivityID, cfg.RoundCount, cfg.RoundDurationSeconds, cfg.RoundIntervalSeconds,
		cfg.TotalBudget, cfg.PerUserRoundCap, cfg.PerUserTotalCap, cfg.BaseUnitAmount, cfg.MaxSingleReward, cfg.ProbabilityStep, now)
	stored, err := scanConfig(row)
	if err != nil {
		return types.RedPacketRainConfig{}, fmt.Errorf("upsert red packet rain config: %w", err)
	}
	return stored, nil
}

// GetRedPacketRainConfig returns the red packet rain rules for one activity.
func (s *Store) GetRedPacketRainConfig(ctx context.Context, activityID int64) (types.RedPacketRainConfig, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+configColumns()+`
		FROM `+s.table("custom_red_packet_rain_configs")+`
		WHERE activity_id = $1
	`, activityID)
	cfg, err := scanConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RedPacketRainConfig{}, types.ErrNotFound
	}
	if err != nil {
		return types.RedPacketRainConfig{}, fmt.Errorf("query red packet rain config: %w", err)
	}
	return cfg, nil
}

// ReplaceRounds replaces pre-generated round windows for an activity in one transaction.
func (s *Store) ReplaceRounds(ctx context.Context, activityID int64, rounds []types.RedPacketRainRound) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace red packet rain rounds: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM `+s.table("custom_red_packet_rain_rounds")+` WHERE activity_id = $1`, activityID); err != nil {
		return fmt.Errorf("delete old red packet rain rounds: %w", err)
	}
	for _, round := range rounds {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO `+s.table("custom_red_packet_rain_rounds")+` (
				activity_id, round_no, starts_at, ends_at, status, created_at
			) VALUES ($1, $2, $3, $4, $5, $6)
		`, activityID, round.RoundNo, round.StartsAt.UTC(), round.EndsAt.UTC(), round.Status, normalizeNow(round.CreatedAt)); err != nil {
			return fmt.Errorf("insert red packet rain round %d: %w", round.RoundNo, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace red packet rain rounds: %w", err)
	}
	return nil
}

// ListRounds returns all pre-generated rounds for an activity.
func (s *Store) ListRounds(ctx context.Context, activityID int64) ([]types.RedPacketRainRound, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+roundColumns()+`
		FROM `+s.table("custom_red_packet_rain_rounds")+`
		WHERE activity_id = $1
		ORDER BY round_no ASC
	`, activityID)
	if err != nil {
		return nil, fmt.Errorf("list red packet rain rounds: %w", err)
	}
	defer rows.Close()
	return scanRoundRows(rows)
}

// GetRound returns a single red packet rain round.
func (s *Store) GetRound(ctx context.Context, roundID int64) (types.RedPacketRainRound, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+roundColumns()+`
		FROM `+s.table("custom_red_packet_rain_rounds")+`
		WHERE id = $1
	`, roundID)
	round, err := scanRound(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RedPacketRainRound{}, types.ErrNotFound
	}
	if err != nil {
		return types.RedPacketRainRound{}, fmt.Errorf("query red packet rain round: %w", err)
	}
	return round, nil
}

// FindRoundAt returns the round whose window contains now.
func (s *Store) FindRoundAt(ctx context.Context, activityID int64, now time.Time) (types.RedPacketRainRound, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+roundColumns()+`
		FROM `+s.table("custom_red_packet_rain_rounds")+`
		WHERE activity_id = $1
		  AND starts_at <= $2
		  AND ends_at > $2
		ORDER BY round_no ASC
		LIMIT 1
	`, activityID, now.UTC())
	round, err := scanRound(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RedPacketRainRound{}, types.ErrNotFound
	}
	if err != nil {
		return types.RedPacketRainRound{}, fmt.Errorf("query active red packet rain round: %w", err)
	}
	return round, nil
}

func normalizeAmountText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "0.00000000"
	}
	return trimmed
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
