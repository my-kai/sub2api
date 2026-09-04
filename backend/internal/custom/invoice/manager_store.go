package invoice

import (
	"context"
	"fmt"

	"github.com/lib/pq"
)

// IsInvoiceManager reports whether an active, non-deleted user is in the
// dedicated invoice management allowlist. System-admin bypass is handled by
// the service layer so this query remains a pure relationship lookup.
func (s *Store) IsInvoiceManager(ctx context.Context, userID int64) (bool, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return false, ErrInvalidInput
	}
	var allowed bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM custom_invoice_managers im
			JOIN users u ON u.id = im.user_id
			WHERE im.user_id = $1 AND u.status = 'active' AND u.deleted_at IS NULL
		)
	`, userID).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check invoice manager access: %w", err)
	}
	return allowed, nil
}

// ListInvoiceManagers returns active users currently authorized to manage invoices.
func (s *Store) ListInvoiceManagers(ctx context.Context) ([]InvoiceManager, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalidInput
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email
		FROM custom_invoice_managers im
		JOIN users u ON u.id = im.user_id
		WHERE u.status = 'active' AND u.deleted_at IS NULL
		ORDER BY LOWER(u.email), u.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list invoice managers: %w", err)
	}
	defer rows.Close()
	managers := make([]InvoiceManager, 0)
	for rows.Next() {
		var manager InvoiceManager
		if err := rows.Scan(&manager.UserID, &manager.Email); err != nil {
			return nil, fmt.Errorf("scan invoice manager: %w", err)
		}
		managers = append(managers, manager)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invoice managers: %w", err)
	}
	return managers, nil
}

// ReplaceInvoiceManagers atomically replaces the allowlist with active users.
// Empty userIDs intentionally clears the list and restores the admin-only default.
func (s *Store) ReplaceInvoiceManagers(ctx context.Context, grantedBy int64, userIDs []int64) ([]InvoiceManager, error) {
	if s == nil || s.db == nil || grantedBy <= 0 {
		return nil, ErrInvalidInput
	}
	uniqueIDs, err := validateManagerIDs(userIDs)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin invoice manager update: %w", err)
	}
	committed := false
	defer rollbackUnlessCommitted(tx, &committed)
	// Serialize whole-list replacements so concurrent administrators cannot
	// interleave DELETE/INSERT operations into a mixed permission set.
	if _, err := tx.ExecContext(ctx, `LOCK TABLE custom_invoice_managers IN EXCLUSIVE MODE`); err != nil {
		return nil, fmt.Errorf("lock invoice manager update: %w", err)
	}

	if len(uniqueIDs) > 0 {
		var activeCount int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM users
			WHERE id = ANY($1) AND status = 'active' AND deleted_at IS NULL
		`, pq.Array(uniqueIDs)).Scan(&activeCount); err != nil {
			return nil, fmt.Errorf("validate invoice manager users: %w", err)
		}
		if activeCount != len(uniqueIDs) {
			return nil, ErrUserNotEligible
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM custom_invoice_managers`); err != nil {
		return nil, fmt.Errorf("clear invoice managers: %w", err)
	}
	for _, userID := range uniqueIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO custom_invoice_managers (user_id, granted_by, created_at)
			VALUES ($1, $2, NOW())
		`, userID, grantedBy); err != nil {
			return nil, fmt.Errorf("insert invoice manager: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit invoice manager update: %w", err)
	}
	committed = true
	return s.ListInvoiceManagers(ctx)
}

func validateManagerIDs(ids []int64) ([]int64, error) {
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalidInput
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique, nil
}
