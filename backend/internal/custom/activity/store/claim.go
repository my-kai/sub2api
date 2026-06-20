package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
)

const activityRewardRedeemType = "activity_reward"

// CreateClaim records one idempotent red packet rain settlement result.
func (s *Store) CreateClaim(ctx context.Context, claim types.RedPacketRainClaim) (types.RedPacketRainClaim, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO `+s.table("custom_red_packet_rain_claims")+` (
			activity_id, round_id, user_id, hit_count, reward_amount, idempotency_key, created_at
		) VALUES ($1, $2, $3, $4, $5::decimal, $6, $7)
		RETURNING `+claimColumns()+`
	`, claim.ActivityID, claim.RoundID, claim.UserID, claim.HitCount, claim.RewardAmount,
		strings.TrimSpace(claim.IdempotencyKey), normalizeNow(claim.CreatedAt))
	stored, err := scanClaim(row)
	if err != nil {
		return types.RedPacketRainClaim{}, fmt.Errorf("create red packet rain claim: %w", err)
	}
	return stored, nil
}

// GetClaimByIdempotencyKey returns the first settlement for a repeated client key.
func (s *Store) GetClaimByIdempotencyKey(ctx context.Context, activityID int64, roundID int64, userID int64, key string) (types.RedPacketRainClaim, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+claimColumns()+`
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1
		  AND round_id = $2
		  AND user_id = $3
		  AND idempotency_key = $4
	`, activityID, roundID, userID, strings.TrimSpace(key))
	claim, err := scanClaim(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RedPacketRainClaim{}, types.ErrNotFound
	}
	if err != nil {
		return types.RedPacketRainClaim{}, fmt.Errorf("query red packet rain idempotent claim: %w", err)
	}
	return claim, nil
}

// ListClaims returns paginated claim records for admin audit views.
func (s *Store) ListClaims(ctx context.Context, activityID int64, page types.PageRequest) ([]types.RedPacketRainClaim, int64, error) {
	page = normalizePage(page)
	var total int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1
	`, activityID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count red packet rain claims: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+claimColumns()+`
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, activityID, page.PageSize, (page.Page-1)*page.PageSize)
	if err != nil {
		return nil, 0, fmt.Errorf("list red packet rain claims: %w", err)
	}
	defer rows.Close()
	items, err := scanClaimRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ClaimSummary returns budget and user cap totals needed by the settlement transaction.
func (s *Store) ClaimSummary(ctx context.Context, activityID int64, roundID int64, userID int64) (types.ClaimSummary, error) {
	var summary types.ClaimSummary
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(reward_amount), 0)::text,
			COUNT(DISTINCT user_id)
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1
	`, activityID).Scan(&summary.ActivityIssuedAmount, &summary.ParticipantCount)
	if err != nil {
		return types.ClaimSummary{}, fmt.Errorf("query red packet rain activity summary: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(reward_amount), 0)::text
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1 AND round_id = $2 AND user_id = $3
	`, activityID, roundID, userID).Scan(&summary.UserRoundAmount); err != nil {
		return types.ClaimSummary{}, fmt.Errorf("query red packet rain user round summary: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(reward_amount), 0)::text
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1 AND user_id = $2
	`, activityID, userID).Scan(&summary.UserActivityAmount); err != nil {
		return types.ClaimSummary{}, fmt.Errorf("query red packet rain user activity summary: %w", err)
	}
	return summary, nil
}

// RoundClaimSummary is the aggregate shown in admin round tables.
type RoundClaimSummary struct {
	RoundID          int64
	IssuedAmount     string
	ParticipantCount int64
	ClaimCount       int64
}

// RoundClaimSummaries returns per-round aggregates for admin details.
func (s *Store) RoundClaimSummaries(ctx context.Context, activityID int64) (map[int64]RoundClaimSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			round_id,
			COALESCE(SUM(reward_amount), 0)::text,
			COUNT(DISTINCT user_id),
			COUNT(*)
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1
		GROUP BY round_id
	`, activityID)
	if err != nil {
		return nil, fmt.Errorf("query red packet rain round summaries: %w", err)
	}
	defer rows.Close()

	result := map[int64]RoundClaimSummary{}
	for rows.Next() {
		var item RoundClaimSummary
		if err := rows.Scan(&item.RoundID, &item.IssuedAmount, &item.ParticipantCount, &item.ClaimCount); err != nil {
			return nil, fmt.Errorf("scan red packet rain round summary: %w", err)
		}
		result[item.RoundID] = item
	}
	return result, rows.Err()
}

// ClaimTransactionInput carries all values that must be settled atomically.
type ClaimTransactionInput struct {
	ActivityID        int64
	RoundID           int64
	UserID            int64
	HitCount          int
	IdempotencyKey    string
	RewardAmount      string
	ActivityTitle     string
	CreatedAt         time.Time
	CreditUserBalance bool
}

// ClaimTransactionResult returns the inserted or previously-settled claim and fresh totals.
type ClaimTransactionResult struct {
	Claim     types.RedPacketRainClaim
	Summary   types.ClaimSummary
	Duplicate bool
}

// ClaimRewardDecision is produced after fresh totals have been read inside the transaction.
type ClaimRewardDecision struct {
	RewardAmount      string
	CreditUserBalance bool
}

// ClaimRewardDecider calculates a final reward using totals protected by the activity lock.
type ClaimRewardDecider func(summary types.ClaimSummary) (ClaimRewardDecision, error)

// SettleClaim serializes one activity's settlement state in SQL.
//
// PostgreSQL advisory locks are intentionally scoped to the whole activity
// rather than only activity+user. The total budget is shared by all accounts,
// so cross-user claims must be serialized before the reward is capped.
func (s *Store) SettleClaim(ctx context.Context, input ClaimTransactionInput, decide ClaimRewardDecider) (ClaimTransactionResult, error) {
	if s == nil || s.db == nil {
		return ClaimTransactionResult{}, fmt.Errorf("custom activity store is not configured")
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" || input.ActivityID <= 0 || input.RoundID <= 0 || input.UserID <= 0 || input.HitCount < 0 || decide == nil {
		return ClaimTransactionResult{}, types.ErrInvalidInput
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimTransactionResult{}, fmt.Errorf("begin red packet rain claim transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockSettlement(ctx, tx, input.ActivityID); err != nil {
		return ClaimTransactionResult{}, err
	}

	existing, err := s.getClaimByIdempotencyKeyWithExecutor(ctx, tx, input.ActivityID, input.RoundID, input.UserID, key)
	if err == nil {
		summary, summaryErr := s.claimSummaryWithExecutor(ctx, tx, input.ActivityID, input.RoundID, input.UserID)
		if summaryErr != nil {
			return ClaimTransactionResult{}, summaryErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return ClaimTransactionResult{}, fmt.Errorf("commit duplicate red packet rain claim: %w", commitErr)
		}
		return ClaimTransactionResult{Claim: existing, Summary: summary, Duplicate: true}, nil
	}
	if !errors.Is(err, types.ErrNotFound) {
		return ClaimTransactionResult{}, err
	}

	summary, err := s.claimSummaryWithExecutor(ctx, tx, input.ActivityID, input.RoundID, input.UserID)
	if err != nil {
		return ClaimTransactionResult{}, err
	}
	decision, err := decide(summary)
	if err != nil {
		return ClaimTransactionResult{}, err
	}

	createdAt := normalizeNow(input.CreatedAt)
	claim, err := s.createClaimWithExecutor(ctx, tx, types.RedPacketRainClaim{
		ActivityID:     input.ActivityID,
		RoundID:        input.RoundID,
		UserID:         input.UserID,
		HitCount:       input.HitCount,
		RewardAmount:   normalizeAmountText(decision.RewardAmount),
		IdempotencyKey: key,
		CreatedAt:      createdAt,
	})
	if err != nil {
		return ClaimTransactionResult{}, err
	}

	input.RewardAmount = normalizeAmountText(decision.RewardAmount)
	if decision.CreditUserBalance && input.RewardAmount != "0.00000000" {
		if err := addUserBalance(ctx, tx, input.UserID, input.RewardAmount); err != nil {
			return ClaimTransactionResult{}, err
		}
		if err := insertActivityBalanceHistory(ctx, tx, input, claim.ID, createdAt); err != nil {
			return ClaimTransactionResult{}, err
		}
	}

	summary, err = s.claimSummaryWithExecutor(ctx, tx, input.ActivityID, input.RoundID, input.UserID)
	if err != nil {
		return ClaimTransactionResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ClaimTransactionResult{}, fmt.Errorf("commit red packet rain claim transaction: %w", err)
	}
	return ClaimTransactionResult{Claim: claim, Summary: summary}, nil
}

// CreateClaimAndCreditBalance keeps a narrow fixed-reward helper for tests and future maintenance.
//
// Callers that enforce caps must prefer SettleClaim so final rewards are based
// on totals read after the activity-level transaction lock has been acquired.
func (s *Store) CreateClaimAndCreditBalance(ctx context.Context, input ClaimTransactionInput) (ClaimTransactionResult, error) {
	return s.SettleClaim(ctx, input, func(types.ClaimSummary) (ClaimRewardDecision, error) {
		return ClaimRewardDecision{
			RewardAmount:      normalizeAmountText(input.RewardAmount),
			CreditUserBalance: input.CreditUserBalance,
		}, nil
	})
}

func (s *Store) createClaimWithExecutor(ctx context.Context, exec sqlExecutor, claim types.RedPacketRainClaim) (types.RedPacketRainClaim, error) {
	row := exec.QueryRowContext(ctx, `
		INSERT INTO `+s.table("custom_red_packet_rain_claims")+` (
			activity_id, round_id, user_id, hit_count, reward_amount, idempotency_key, created_at
		) VALUES ($1, $2, $3, $4, $5::decimal, $6, $7)
		RETURNING `+claimColumns()+`
	`, claim.ActivityID, claim.RoundID, claim.UserID, claim.HitCount, normalizeAmountText(claim.RewardAmount),
		strings.TrimSpace(claim.IdempotencyKey), normalizeNow(claim.CreatedAt))
	stored, err := scanClaim(row)
	if err != nil {
		return types.RedPacketRainClaim{}, fmt.Errorf("create red packet rain claim: %w", err)
	}
	return stored, nil
}

func (s *Store) getClaimByIdempotencyKeyWithExecutor(ctx context.Context, exec sqlExecutor, activityID int64, roundID int64, userID int64, key string) (types.RedPacketRainClaim, error) {
	row := exec.QueryRowContext(ctx, `
		SELECT `+claimColumns()+`
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1
		  AND round_id = $2
		  AND user_id = $3
		  AND idempotency_key = $4
	`, activityID, roundID, userID, strings.TrimSpace(key))
	claim, err := scanClaim(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.RedPacketRainClaim{}, types.ErrNotFound
	}
	if err != nil {
		return types.RedPacketRainClaim{}, fmt.Errorf("query red packet rain idempotent claim: %w", err)
	}
	return claim, nil
}

func (s *Store) claimSummaryWithExecutor(ctx context.Context, exec sqlExecutor, activityID int64, roundID int64, userID int64) (types.ClaimSummary, error) {
	var summary types.ClaimSummary
	if err := exec.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(reward_amount), 0)::text,
			COUNT(DISTINCT user_id)
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1
	`, activityID).Scan(&summary.ActivityIssuedAmount, &summary.ParticipantCount); err != nil {
		return types.ClaimSummary{}, fmt.Errorf("query red packet rain activity summary: %w", err)
	}
	if err := exec.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(reward_amount), 0)::text
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1 AND round_id = $2 AND user_id = $3
	`, activityID, roundID, userID).Scan(&summary.UserRoundAmount); err != nil {
		return types.ClaimSummary{}, fmt.Errorf("query red packet rain user round summary: %w", err)
	}
	if err := exec.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(reward_amount), 0)::text
		FROM `+s.table("custom_red_packet_rain_claims")+`
		WHERE activity_id = $1 AND user_id = $2
	`, activityID, userID).Scan(&summary.UserActivityAmount); err != nil {
		return types.ClaimSummary{}, fmt.Errorf("query red packet rain user activity summary: %w", err)
	}
	return summary, nil
}

func lockSettlement(ctx context.Context, exec sqlExecutor, activityID int64) error {
	if _, err := exec.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, activityID); err != nil {
		return fmt.Errorf("lock red packet rain settlement: %w", err)
	}
	return nil
}

func addUserBalance(ctx context.Context, exec sqlExecutor, userID int64, amount string) error {
	result, err := exec.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + $1::decimal,
		    total_recharged = total_recharged + $1::decimal,
		    updated_at = NOW()
		WHERE id = $2
		  AND deleted_at IS NULL
	`, amount, userID)
	if err != nil {
		return fmt.Errorf("credit red packet rain balance: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("credit red packet rain balance: user not found")
	}
	return nil
}

func insertActivityBalanceHistory(ctx context.Context, exec sqlExecutor, input ClaimTransactionInput, claimID int64, at time.Time) error {
	note := fmt.Sprintf("红包雨奖励：%s", strings.TrimSpace(input.ActivityTitle))
	if strings.TrimSpace(input.ActivityTitle) == "" {
		note = "红包雨奖励"
	}
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO redeem_codes (
			code, type, value, status, used_by, used_at, notes, created_at, validity_days
		) VALUES (
			$1, $2, $3::decimal, $4, $5, $6, $7, $6, 0
		)
	`, activityBalanceHistoryCode(input.ActivityID, input.RoundID, input.UserID, claimID, input.IdempotencyKey),
		activityRewardRedeemType, normalizeAmountText(input.RewardAmount), "used", input.UserID, at.UTC(), note); err != nil {
		return fmt.Errorf("insert red packet rain balance history: %w", err)
	}
	return nil
}

func activityBalanceHistoryCode(activityID int64, roundID int64, userID int64, claimID int64, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("activity:%d:%d:%d:%d:%s", activityID, roundID, userID, claimID, idempotencyKey)))
	// redeem_codes.code is limited to 32 chars. AR keeps history searchable without widening main schema.
	return "AR" + strings.ToUpper(hex.EncodeToString(sum[:15]))
}
