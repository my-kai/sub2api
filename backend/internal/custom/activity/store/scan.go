package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
)

func (s *Store) table(name string) string {
	return s.tablePrefix + name
}

func activityColumns() string {
	return `id, type, title, description, cover_url, status, starts_at, ends_at, created_by, created_at, updated_at, ended_at`
}

func prefixedActivityColumns(alias string) string {
	columns := []string{
		"id", "type", "title", "description", "cover_url", "status",
		"starts_at", "ends_at", "created_by", "created_at", "updated_at", "ended_at",
	}
	for i := range columns {
		columns[i] = alias + "." + columns[i]
	}
	return strings.Join(columns, ", ")
}

func configColumns() string {
	return `activity_id, round_count, round_duration_seconds, round_interval_seconds,
		total_budget::text, per_user_round_cap::text, per_user_total_cap::text,
		base_unit_amount::text, max_single_reward::text, probability_step::text,
		created_at, updated_at`
}

func roundColumns() string {
	return `id, activity_id, round_no, starts_at, ends_at, status, created_at`
}

func claimColumns() string {
	return `id, activity_id, round_id, user_id, hit_count, reward_amount::text, idempotency_key, created_at`
}

func wsTicketColumns() string {
	return `id, ticket_hash, activity_id, round_id, user_id, device_fingerprint, client_nonce, expires_at, consumed_at, created_at`
}

func wsSessionColumns() string {
	return `id, session_id, activity_id, round_id, user_id, device_fingerprint, client_nonce, server_nonce,
		challenge_hash, used_nonces::text, risk_status, risk_reason, expires_at, created_at, closed_at`
}

func scanActivity(row interface{ Scan(dest ...any) error }) (types.Activity, error) {
	var item types.Activity
	var createdBy sql.NullInt64
	var endedAt sql.NullTime
	if err := row.Scan(
		&item.ID,
		&item.Type,
		&item.Title,
		&item.Description,
		&item.CoverURL,
		&item.Status,
		&item.StartsAt,
		&item.EndsAt,
		&createdBy,
		&item.CreatedAt,
		&item.UpdatedAt,
		&endedAt,
	); err != nil {
		return types.Activity{}, err
	}
	item.CreatedBy = nullInt64(createdBy)
	item.StartsAt = item.StartsAt.UTC()
	item.EndsAt = item.EndsAt.UTC()
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	if endedAt.Valid {
		value := endedAt.Time.UTC()
		item.EndedAt = &value
	}
	return item, nil
}

func scanActivityRows(rows *sql.Rows) ([]types.Activity, error) {
	items := []types.Activity{}
	for rows.Next() {
		item, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanAdminSummary(row interface{ Scan(dest ...any) error }) (types.ActivityAdminSummary, error) {
	var summary types.ActivityAdminSummary
	var createdBy sql.NullInt64
	var endedAt sql.NullTime
	if err := row.Scan(
		&summary.Activity.ID,
		&summary.Activity.Type,
		&summary.Activity.Title,
		&summary.Activity.Description,
		&summary.Activity.CoverURL,
		&summary.Activity.Status,
		&summary.Activity.StartsAt,
		&summary.Activity.EndsAt,
		&createdBy,
		&summary.Activity.CreatedAt,
		&summary.Activity.UpdatedAt,
		&endedAt,
		&summary.TotalBudget,
		&summary.IssuedAmount,
		&summary.ParticipantCount,
	); err != nil {
		return types.ActivityAdminSummary{}, fmt.Errorf("scan custom activity admin summary: %w", err)
	}
	summary.Activity.CreatedBy = nullInt64(createdBy)
	summary.Activity.StartsAt = summary.Activity.StartsAt.UTC()
	summary.Activity.EndsAt = summary.Activity.EndsAt.UTC()
	summary.Activity.CreatedAt = summary.Activity.CreatedAt.UTC()
	summary.Activity.UpdatedAt = summary.Activity.UpdatedAt.UTC()
	if endedAt.Valid {
		value := endedAt.Time.UTC()
		summary.Activity.EndedAt = &value
	}
	return summary, nil
}

func scanConfig(row interface{ Scan(dest ...any) error }) (types.RedPacketRainConfig, error) {
	var cfg types.RedPacketRainConfig
	if err := row.Scan(
		&cfg.ActivityID,
		&cfg.RoundCount,
		&cfg.RoundDurationSeconds,
		&cfg.RoundIntervalSeconds,
		&cfg.TotalBudget,
		&cfg.PerUserRoundCap,
		&cfg.PerUserTotalCap,
		&cfg.BaseUnitAmount,
		&cfg.MaxSingleReward,
		&cfg.ProbabilityStep,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	); err != nil {
		return types.RedPacketRainConfig{}, err
	}
	cfg.CreatedAt = cfg.CreatedAt.UTC()
	cfg.UpdatedAt = cfg.UpdatedAt.UTC()
	return cfg, nil
}

func scanRound(row interface{ Scan(dest ...any) error }) (types.RedPacketRainRound, error) {
	var round types.RedPacketRainRound
	if err := row.Scan(
		&round.ID,
		&round.ActivityID,
		&round.RoundNo,
		&round.StartsAt,
		&round.EndsAt,
		&round.Status,
		&round.CreatedAt,
	); err != nil {
		return types.RedPacketRainRound{}, err
	}
	round.StartsAt = round.StartsAt.UTC()
	round.EndsAt = round.EndsAt.UTC()
	round.CreatedAt = round.CreatedAt.UTC()
	return round, nil
}

func scanRoundRows(rows *sql.Rows) ([]types.RedPacketRainRound, error) {
	items := []types.RedPacketRainRound{}
	for rows.Next() {
		item, err := scanRound(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanClaim(row interface{ Scan(dest ...any) error }) (types.RedPacketRainClaim, error) {
	var claim types.RedPacketRainClaim
	if err := row.Scan(
		&claim.ID,
		&claim.ActivityID,
		&claim.RoundID,
		&claim.UserID,
		&claim.HitCount,
		&claim.RewardAmount,
		&claim.IdempotencyKey,
		&claim.CreatedAt,
	); err != nil {
		return types.RedPacketRainClaim{}, err
	}
	claim.CreatedAt = claim.CreatedAt.UTC()
	return claim, nil
}

func scanClaimRows(rows *sql.Rows) ([]types.RedPacketRainClaim, error) {
	items := []types.RedPacketRainClaim{}
	for rows.Next() {
		item, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanWSTicket(row interface{ Scan(dest ...any) error }) (types.RedPacketRainWSTicket, error) {
	var ticket types.RedPacketRainWSTicket
	var consumedAt sql.NullTime
	if err := row.Scan(
		&ticket.ID,
		&ticket.TicketHash,
		&ticket.ActivityID,
		&ticket.RoundID,
		&ticket.UserID,
		&ticket.DeviceFingerprint,
		&ticket.ClientNonce,
		&ticket.ExpiresAt,
		&consumedAt,
		&ticket.CreatedAt,
	); err != nil {
		return types.RedPacketRainWSTicket{}, err
	}
	ticket.ExpiresAt = ticket.ExpiresAt.UTC()
	ticket.CreatedAt = ticket.CreatedAt.UTC()
	if consumedAt.Valid {
		value := consumedAt.Time.UTC()
		ticket.ConsumedAt = &value
	}
	return ticket, nil
}

func scanWSSession(row interface{ Scan(dest ...any) error }) (types.RedPacketRainWSSession, error) {
	var session types.RedPacketRainWSSession
	var usedNonces string
	var closedAt sql.NullTime
	if err := row.Scan(
		&session.ID,
		&session.SessionID,
		&session.ActivityID,
		&session.RoundID,
		&session.UserID,
		&session.DeviceFingerprint,
		&session.ClientNonce,
		&session.ServerNonce,
		&session.ChallengeHash,
		&usedNonces,
		&session.RiskStatus,
		&session.RiskReason,
		&session.ExpiresAt,
		&session.CreatedAt,
		&closedAt,
	); err != nil {
		return types.RedPacketRainWSSession{}, err
	}
	if strings.TrimSpace(usedNonces) != "" {
		if err := json.Unmarshal([]byte(usedNonces), &session.UsedNonces); err != nil {
			return types.RedPacketRainWSSession{}, fmt.Errorf("scan websocket used nonces: %w", err)
		}
	}
	session.ExpiresAt = session.ExpiresAt.UTC()
	session.CreatedAt = session.CreatedAt.UTC()
	if closedAt.Valid {
		value := closedAt.Time.UTC()
		session.ClosedAt = &value
	}
	return session, nil
}

func validateTablePrefix(prefix string) error {
	for _, r := range strings.TrimSpace(prefix) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return fmt.Errorf("custom activity table prefix is invalid")
	}
	return nil
}

func normalizeNow(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func nullInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullInt64Param(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullTimeParam(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

func normalizePage(page types.PageRequest) types.PageRequest {
	if page.Page <= 0 {
		page.Page = 1
	}
	if page.PageSize <= 0 || page.PageSize > 50 {
		page.PageSize = 20
	}
	return page
}
