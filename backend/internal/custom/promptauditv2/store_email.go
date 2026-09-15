package promptauditv2

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ClaimEmailJob leases one due job with SKIP LOCKED so multiple instances can
// share the persistent queue without delivering the same event concurrently.
func (s *Store) ClaimEmailJob(ctx context.Context, lease time.Duration) (*EmailJob, error) {
	if lease <= 0 {
		return nil, fmt.Errorf("prompt audit v2 email lease must be positive")
	}
	var job EmailJob
	err := s.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id FROM custom_prompt_audit_v2_email_jobs
			WHERE (status = 'pending' AND available_at <= NOW())
			   OR (status = 'processing' AND lease_until < NOW())
			ORDER BY id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE custom_prompt_audit_v2_email_jobs AS job
		SET status = 'processing', attempts = attempts + 1,
			lease_until = NOW() + ($1 * INTERVAL '1 second'), updated_at = NOW()
		FROM candidate
		WHERE job.id = candidate.id
		RETURNING job.id, job.event_id, job.recipient_email, job.attempts
	`, int64(lease/time.Second)).Scan(&job.ID, &job.EventID, &job.RecipientEmail, &job.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim prompt audit v2 email job: %w", err)
	}
	event, err := s.GetEvent(ctx, job.EventID)
	if err != nil {
		return nil, fmt.Errorf("load prompt audit v2 email event: %w", err)
	}
	job.Event = *event
	return &job, nil
}

// CompleteEmailJob marks a leased delivery as sent.
func (s *Store) CompleteEmailJob(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE custom_prompt_audit_v2_email_jobs
		SET status = 'sent', lease_until = NULL, sent_at = NOW(), updated_at = NOW(), last_error_code = ''
		WHERE id = $1 AND status = 'processing'
	`, id)
	if err != nil {
		return fmt.Errorf("complete prompt audit v2 email job: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("complete prompt audit v2 email job: lease is no longer active")
	}
	return nil
}

// FailEmailJob schedules a bounded retry or records a terminal failure.
func (s *Store) FailEmailJob(ctx context.Context, id int64, attempts, maxAttempts int, retryAt time.Time, code string) error {
	status := "pending"
	if attempts >= maxAttempts {
		status = "failed"
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE custom_prompt_audit_v2_email_jobs
		SET status = $2, available_at = $3, lease_until = NULL,
			last_error_code = $4, updated_at = NOW()
		WHERE id = $1 AND status = 'processing'
	`, id, status, retryAt, code)
	if err != nil {
		return fmt.Errorf("fail prompt audit v2 email job: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return fmt.Errorf("fail prompt audit v2 email job: lease is no longer active")
	}
	return nil
}

// EmailQueueCounts returns persistent pending and terminal-failure totals.
func (s *Store) EmailQueueCounts(ctx context.Context) (int64, int64, error) {
	var pending int64
	var failed int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status IN ('pending', 'processing')),
			COUNT(*) FILTER (WHERE status = 'failed')
		FROM custom_prompt_audit_v2_email_jobs
	`).Scan(&pending, &failed); err != nil {
		return 0, 0, fmt.Errorf("count prompt audit v2 email jobs: %w", err)
	}
	return pending, failed, nil
}

// DeleteExpiredEvents enforces the configured hit-log retention. Email jobs
// cascade with events, while long-lived restrictions retain a nullable source ID.
func (s *Store) DeleteExpiredEvents(ctx context.Context, before time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM custom_prompt_audit_v2_events WHERE created_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("delete expired prompt audit v2 events: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted prompt audit v2 event count: %w", err)
	}
	return count, nil
}
