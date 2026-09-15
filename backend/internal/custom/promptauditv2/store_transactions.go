package promptauditv2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// RecordHit serializes one user's rolling counter, then commits the hit event,
// delivery job, and any reached enforcement action in one database transaction.
func (s *Store) RecordHit(ctx context.Context, input HitInput) (HitOutcome, error) {
	if input.UserID <= 0 || input.Message == "" {
		return HitOutcome{}, fmt.Errorf("prompt audit v2 hit input is incomplete")
	}
	messageCiphertext, err := s.encryptor.Encrypt(input.Message)
	if err != nil {
		return HitOutcome{}, fmt.Errorf("encrypt prompt audit v2 hit message: %w", err)
	}
	messageHash := sha256.Sum256([]byte(input.Message))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HitOutcome{}, fmt.Errorf("begin prompt audit v2 hit transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	lockKey := fmt.Sprintf("prompt-audit-v2:%d", input.UserID)
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return HitOutcome{}, fmt.Errorf("lock prompt audit v2 rolling counter: %w", err)
	}
	// Timestamp after lock acquisition so concurrent requests cannot commit a
	// later event that falls beyond this transaction's rolling-window endpoint.
	recordedAt := time.Now().UTC()
	windowStart := recordedAt.Add(-time.Duration(input.Rule.WindowMinutes) * time.Minute)
	// The existing user-management unban changes users.status back to active.
	// Remove the stale mirror before applying a later rule so it cannot mask a
	// new warning or ban action.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM custom_prompt_audit_v2_restrictions AS restriction
		USING users AS account_user
		WHERE restriction.user_id = $1
		  AND restriction.user_id = account_user.id
		  AND restriction.action = 'ban'
		  AND account_user.status = 'active'
	`, input.UserID); err != nil {
		return HitOutcome{}, fmt.Errorf("remove stale prompt audit v2 ban mirror: %w", err)
	}
	var previousHits int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM custom_prompt_audit_v2_events
		WHERE user_id = $1 AND created_at >= $2 AND created_at <= $3
	`, input.UserID, windowStart, recordedAt).Scan(&previousHits); err != nil {
		return HitOutcome{}, fmt.Errorf("count prompt audit v2 rolling hits: %w", err)
	}

	outcome := HitOutcome{WindowHitCount: previousHits + 1}
	outcome.ThresholdReached = outcome.WindowHitCount >= input.Rule.TriggerCount
	finalAction := "none"
	actionResult := "not_triggered"
	if outcome.ThresholdReached {
		finalAction = input.Rule.Action
		actionResult = "applied"
		outcome.Action = input.Rule.Action
		if input.Rule.Action == ActionWarning {
			if input.Rule.RestrictionMinutes == nil {
				return HitOutcome{}, fmt.Errorf("warning rule has no restriction duration")
			}
			blockedUntil := recordedAt.Add(time.Duration(*input.Rule.RestrictionMinutes) * time.Minute)
			outcome.BlockedUntil = &blockedUntil
		}
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO custom_prompt_audit_v2_events (
			request_id, user_id, username_snapshot, user_email_snapshot,
			api_key_id, api_key_name_snapshot, protocol, request_model,
			endpoint_id, endpoint_name_snapshot, audit_model, confidence, reason, latency_ms,
			rule_threshold, rule_window_minutes, rule_trigger_count,
			rule_action, rule_restriction_minutes, window_hit_count, threshold_reached,
			final_action, action_result, message_ciphertext, message_sha256, message_chars, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27
		) RETURNING id
	`, input.RequestID, input.UserID, input.Username, input.UserEmail,
		input.APIKeyID, input.APIKeyName, input.Protocol, input.RequestModel,
		input.Result.EndpointID, input.Result.EndpointName, input.Result.AuditModel,
		input.Result.Confidence, input.Result.Reason, input.Result.LatencyMS,
		input.Rule.ConfidenceThreshold, input.Rule.WindowMinutes,
		input.Rule.TriggerCount, input.Rule.Action, input.Rule.RestrictionMinutes,
		outcome.WindowHitCount, outcome.ThresholdReached, finalAction, actionResult,
		messageCiphertext, hex.EncodeToString(messageHash[:]), len([]rune(input.Message)), recordedAt,
	).Scan(&outcome.EventID)
	if err != nil {
		return HitOutcome{}, fmt.Errorf("insert prompt audit v2 event: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO custom_prompt_audit_v2_email_jobs (event_id, recipient_email)
		VALUES ($1, $2)
	`, outcome.EventID, input.UserEmail); err != nil {
		return HitOutcome{}, fmt.Errorf("insert prompt audit v2 email job: %w", err)
	}
	if outcome.ThresholdReached {
		if input.Rule.Action == ActionBan {
			result, err := tx.ExecContext(ctx, `UPDATE users SET status = 'disabled', updated_at = NOW() WHERE id = $1`, input.UserID)
			if err != nil {
				return HitOutcome{}, fmt.Errorf("disable prompt audit v2 user: %w", err)
			}
			affected, err := result.RowsAffected()
			if err != nil || affected != 1 {
				return HitOutcome{}, fmt.Errorf("disable prompt audit v2 user: user not found")
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO custom_prompt_audit_v2_restrictions
				(user_id, event_id, action, started_at, blocked_until, updated_at)
			VALUES ($1, $2, $3, $4, $5, NOW())
			ON CONFLICT (user_id) DO UPDATE SET
				event_id = EXCLUDED.event_id,
				action = CASE
					WHEN custom_prompt_audit_v2_restrictions.action = 'ban' THEN 'ban'
					ELSE EXCLUDED.action
				END,
				started_at = CASE
					WHEN custom_prompt_audit_v2_restrictions.action = 'ban' THEN custom_prompt_audit_v2_restrictions.started_at
					ELSE EXCLUDED.started_at
				END,
				blocked_until = CASE
					WHEN custom_prompt_audit_v2_restrictions.action = 'ban' OR EXCLUDED.action = 'ban' THEN NULL
					ELSE GREATEST(custom_prompt_audit_v2_restrictions.blocked_until, EXCLUDED.blocked_until)
				END,
				updated_at = NOW()
		`, input.UserID, outcome.EventID, input.Rule.Action, recordedAt, outcome.BlockedUntil); err != nil {
			return HitOutcome{}, fmt.Errorf("upsert prompt audit v2 restriction: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return HitOutcome{}, fmt.Errorf("commit prompt audit v2 hit: %w", err)
	}
	return outcome, nil
}
