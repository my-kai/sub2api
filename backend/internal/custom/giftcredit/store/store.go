package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/types"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
)

const (
	amountScale               = 8
	giftCreditDeductBatchSize = 16
)

// SQLExecutor is the minimal interface shared by *sql.DB and *sql.Tx.
type SQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store owns all SQL persistence for custom gift credit.
type Store struct {
	db          *sql.DB
	tablePrefix string
}

// NewStore creates a PostgreSQL-backed gift-credit store.
func NewStore(db *sql.DB, tablePrefix string) (*Store, error) {
	if db == nil {
		return nil, errors.New("gift credit store requires sql db")
	}
	if err := validateIdentifierPart(tablePrefix, true); err != nil {
		return nil, fmt.Errorf("gift credit table prefix is invalid: %w", err)
	}
	return &Store{db: db, tablePrefix: strings.TrimSpace(tablePrefix)}, nil
}

// DB exposes the underlying DB for thin callers that need transaction control.
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

// CreateGrant inserts a new grant and refreshes the O(1) user balance aggregate.
//
// The aggregate update happens in the same transaction as the grant insert so
// AI request eligibility can rely on custom_gift_credit_user_balances without
// scanning grant detail rows.
func (s *Store) CreateGrant(ctx context.Context, input types.CreateGrantInput) (types.Grant, error) {
	if s == nil || s.db == nil {
		return types.Grant{}, errors.New("gift credit store is not configured")
	}
	if err := validateCreateGrantInput(input); err != nil {
		return types.Grant{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return types.Grant{}, fmt.Errorf("begin gift credit grant transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	grant, err := s.createGrantWithExecutor(ctx, tx, input)
	if err != nil {
		return types.Grant{}, err
	}
	if err := s.upsertUserBalanceDelta(ctx, tx, input.UserID, normalizeAmount(input.Amount), input.ExpiresAt, input.CreatedAt); err != nil {
		return types.Grant{}, err
	}
	if err := tx.Commit(); err != nil {
		return types.Grant{}, fmt.Errorf("commit gift credit grant transaction: %w", err)
	}
	return grant, nil
}

// UserBalance returns the aggregate balance, lazily refreshing expired grants when needed.
func (s *Store) UserBalance(ctx context.Context, userID int64, now time.Time) (types.UserBalance, error) {
	if s == nil || s.db == nil {
		return types.UserBalance{}, errors.New("gift credit store is not configured")
	}
	if userID <= 0 {
		return types.UserBalance{}, types.ErrInvalidInput
	}
	now = normalizeNow(now)
	balance, err := s.userBalanceWithExecutor(ctx, s.db, userID)
	if err != nil {
		return types.UserBalance{}, err
	}
	if balance.NextExpiresAt != nil && !balance.NextExpiresAt.After(now) {
		if err := s.RefreshUserBalance(ctx, userID, now); err != nil {
			return types.UserBalance{}, err
		}
		return s.userBalanceWithExecutor(ctx, s.db, userID)
	}
	return balance, nil
}

// UserBalances returns aggregate balances for an admin page without N+1 grant scans.
//
// Expired aggregate rows are refreshed explicitly before returning so callers do
// not display stale gift credit after the nearest grant has passed its expiry.
func (s *Store) UserBalances(ctx context.Context, userIDs []int64, now time.Time) (map[int64]types.UserBalance, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("gift credit store is not configured")
	}
	ids := normalizeUserIDs(userIDs)
	if len(ids) == 0 {
		return map[int64]types.UserBalance{}, nil
	}
	now = normalizeNow(now)
	balances, err := s.userBalancesWithExecutor(ctx, s.db, ids)
	if err != nil {
		return nil, err
	}
	expiredUserIDs := make([]int64, 0)
	for _, userID := range ids {
		balance, ok := balances[userID]
		if ok && balance.NextExpiresAt != nil && !balance.NextExpiresAt.After(now) {
			expiredUserIDs = append(expiredUserIDs, userID)
		}
	}
	for _, userID := range expiredUserIDs {
		if err := s.RefreshUserBalance(ctx, userID, now); err != nil {
			return nil, err
		}
	}
	if len(expiredUserIDs) > 0 {
		balances, err = s.userBalancesWithExecutor(ctx, s.db, ids)
		if err != nil {
			return nil, err
		}
	}
	for _, userID := range ids {
		if _, ok := balances[userID]; !ok {
			balances[userID] = zeroUserBalance(userID, now)
		}
	}
	return balances, nil
}

// RefreshUserBalance expires stale grants and recomputes the aggregate for one user.
func (s *Store) RefreshUserBalance(ctx context.Context, userID int64, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("gift credit store is not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin gift credit refresh transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.refreshUserBalanceWithExecutor(ctx, tx, userID, normalizeNow(now)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit gift credit refresh transaction: %w", err)
	}
	return nil
}

// DeductFirst consumes unexpired gift credit before the caller runs normal balance deduction.
//
// The function only owns gift-credit rows. It intentionally returns the
// remaining cost instead of touching users.balance so the existing balance
// deduction concurrency and idempotency logic can remain unchanged.
func (s *Store) DeductFirst(ctx context.Context, exec SQLExecutor, input types.DeductInput) (types.DeductResult, error) {
	if s == nil {
		return types.DeductResult{}, errors.New("gift credit store is not configured")
	}
	if exec == nil {
		exec = s.db
	}
	amount, err := parsePositiveAmount(input.Amount)
	if err != nil || input.UserID <= 0 || strings.TrimSpace(input.RequestID) == "" {
		return types.DeductResult{}, types.ErrInvalidInput
	}
	now := normalizeNow(input.Now)
	balance, err := s.userBalanceWithExecutor(ctx, exec, input.UserID)
	if err != nil {
		return types.DeductResult{}, err
	}
	if balance.NextExpiresAt != nil && !balance.NextExpiresAt.After(now) {
		if err := s.refreshUserBalanceWithExecutor(ctx, exec, input.UserID, now); err != nil {
			return types.DeductResult{}, err
		}
		balance, err = s.userBalanceWithExecutor(ctx, exec, input.UserID)
		if err != nil {
			return types.DeductResult{}, err
		}
	}
	active, err := parseAmount(balance.ActiveRemainingAmount)
	if err != nil {
		return types.DeductResult{}, err
	}
	if active.LessThanOrEqual(decimal.Zero) {
		return types.DeductResult{
			GiftDeducted:       normalizeAmountDecimal(decimal.Zero),
			RemainingCost:      normalizeAmountDecimal(amount),
			NewGiftBalance:     normalizeAmountDecimal(decimal.Zero),
			SkippedGrantLookup: true,
		}, nil
	}
	remaining := amount
	deductedTotal := decimal.Zero
	deductions := make([]types.Deduction, 0, 1)
	for remaining.GreaterThan(decimal.Zero) && deductedTotal.LessThan(active) {
		grants, err := s.lockUsableGrants(ctx, exec, input.UserID, now, giftCreditDeductBatchSize)
		if err != nil {
			return types.DeductResult{}, err
		}
		if len(grants) == 0 {
			break
		}
		for _, grant := range grants {
			if remaining.LessThanOrEqual(decimal.Zero) {
				break
			}
			grantRemaining, err := parseAmount(grant.RemainingAmount)
			if err != nil {
				return types.DeductResult{}, err
			}
			deductAmount := decimal.Min(remaining, grantRemaining)
			if deductAmount.LessThanOrEqual(decimal.Zero) {
				continue
			}
			deduction, err := s.applyGrantDeduction(ctx, exec, grant, input, deductAmount, now)
			if err != nil {
				return types.DeductResult{}, err
			}
			deductions = append(deductions, deduction)
			remaining = remaining.Sub(deductAmount)
			deductedTotal = deductedTotal.Add(deductAmount)
		}
		if len(grants) < giftCreditDeductBatchSize {
			break
		}
	}
	if deductedTotal.GreaterThan(decimal.Zero) {
		nextBalance := active.Sub(deductedTotal)
		if nextBalance.LessThan(decimal.Zero) {
			nextBalance = decimal.Zero
		}
		if err := s.setUserBalanceAggregate(ctx, exec, input.UserID, nextBalance, now); err != nil {
			return types.DeductResult{}, err
		}
	}
	updatedBalance, err := s.userBalanceWithExecutor(ctx, exec, input.UserID)
	if err != nil {
		return types.DeductResult{}, err
	}
	return types.DeductResult{
		GiftDeducted:   normalizeAmountDecimal(deductedTotal),
		RemainingCost:  normalizeAmountDecimal(remaining),
		NewGiftBalance: updatedBalance.ActiveRemainingAmount,
		Deductions:     deductions,
	}, nil
}

func (s *Store) createGrantWithExecutor(ctx context.Context, exec SQLExecutor, input types.CreateGrantInput) (types.Grant, error) {
	var expiresAt any
	if input.ExpiresAt != nil {
		expiresAt = input.ExpiresAt.UTC()
	}
	row := exec.QueryRowContext(ctx, `
		INSERT INTO `+s.table("custom_gift_credit_grants")+` (
			user_id, source_type, source_id, original_amount, remaining_amount,
			expires_at, status, note, created_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4::decimal, $4::decimal, $5, $6, $7, $8, $9, $9
		)
		RETURNING `+grantColumns()+`
	`, input.UserID, strings.TrimSpace(input.SourceType), strings.TrimSpace(input.SourceID),
		normalizeAmount(input.Amount), expiresAt, types.StatusActive,
		strings.TrimSpace(input.Note), input.CreatedBy, normalizeNow(input.CreatedAt))
	grant, err := scanGrant(row)
	if err != nil {
		return types.Grant{}, fmt.Errorf("create gift credit grant: %w", err)
	}
	return grant, nil
}

func (s *Store) userBalanceWithExecutor(ctx context.Context, exec SQLExecutor, userID int64) (types.UserBalance, error) {
	row := exec.QueryRowContext(ctx, `
		SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at
		FROM `+s.table("custom_gift_credit_user_balances")+`
		WHERE user_id = $1
	`, userID)
	balance, err := scanUserBalance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return types.UserBalance{
			UserID:                userID,
			ActiveRemainingAmount: normalizeAmountDecimal(decimal.Zero),
			RefreshedAt:           time.Time{},
			UpdatedAt:             time.Time{},
		}, nil
	}
	if err != nil {
		return types.UserBalance{}, fmt.Errorf("query gift credit user balance: %w", err)
	}
	return balance, nil
}

func (s *Store) userBalancesWithExecutor(ctx context.Context, exec SQLExecutor, userIDs []int64) (map[int64]types.UserBalance, error) {
	rows, err := exec.QueryContext(ctx, `
		SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at
		FROM `+s.table("custom_gift_credit_user_balances")+`
		WHERE user_id = ANY($1)
	`, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("query gift credit user balances: %w", err)
	}
	defer rows.Close()
	balances := make(map[int64]types.UserBalance, len(userIDs))
	for rows.Next() {
		balance, err := scanUserBalance(rows)
		if err != nil {
			return nil, err
		}
		balances[balance.UserID] = balance
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return balances, nil
}

func normalizeUserIDs(userIDs []int64) []int64 {
	seen := make(map[int64]struct{}, len(userIDs))
	ids := make([]int64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}
		ids = append(ids, userID)
	}
	return ids
}

func zeroUserBalance(userID int64, now time.Time) types.UserBalance {
	return types.UserBalance{
		UserID:                userID,
		ActiveRemainingAmount: normalizeAmountDecimal(decimal.Zero),
		RefreshedAt:           now.UTC(),
		UpdatedAt:             now.UTC(),
	}
}

func (s *Store) refreshUserBalanceWithExecutor(ctx context.Context, exec SQLExecutor, userID int64, now time.Time) error {
	if userID <= 0 {
		return types.ErrInvalidInput
	}
	if _, err := exec.ExecContext(ctx, `
		UPDATE `+s.table("custom_gift_credit_grants")+`
		SET status = $1,
		    updated_at = $2
		WHERE user_id = $3
		  AND status = $4
		  AND remaining_amount > 0
		  AND expires_at IS NOT NULL
		  AND expires_at <= $2
	`, types.StatusExpired, now.UTC(), userID, types.StatusActive); err != nil {
		return fmt.Errorf("expire gift credit grants: %w", err)
	}
	if err := s.recomputeUserBalance(ctx, exec, userID, now); err != nil {
		return err
	}
	return nil
}

func (s *Store) upsertUserBalanceDelta(ctx context.Context, exec SQLExecutor, userID int64, delta string, expiresAt *time.Time, now time.Time) error {
	if userID <= 0 {
		return types.ErrInvalidInput
	}
	var nextExpires any
	if expiresAt != nil {
		nextExpires = expiresAt.UTC()
	}
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO `+s.table("custom_gift_credit_user_balances")+` (
			user_id, active_remaining_amount, next_expires_at, refreshed_at, updated_at
		) VALUES (
			$1, $2::decimal, $3, $4, $4
		)
		ON CONFLICT (user_id) DO UPDATE SET
			active_remaining_amount = `+s.table("custom_gift_credit_user_balances")+`.active_remaining_amount + EXCLUDED.active_remaining_amount,
			next_expires_at = CASE
				WHEN `+s.table("custom_gift_credit_user_balances")+`.next_expires_at IS NULL THEN EXCLUDED.next_expires_at
				WHEN EXCLUDED.next_expires_at IS NULL THEN `+s.table("custom_gift_credit_user_balances")+`.next_expires_at
				WHEN EXCLUDED.next_expires_at < `+s.table("custom_gift_credit_user_balances")+`.next_expires_at THEN EXCLUDED.next_expires_at
				ELSE `+s.table("custom_gift_credit_user_balances")+`.next_expires_at
			END,
			updated_at = EXCLUDED.updated_at
	`, userID, normalizeAmount(delta), nextExpires, normalizeNow(now)); err != nil {
		return fmt.Errorf("upsert gift credit user balance: %w", err)
	}
	return nil
}

func (s *Store) setUserBalanceAggregate(ctx context.Context, exec SQLExecutor, userID int64, amount decimal.Decimal, now time.Time) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE `+s.table("custom_gift_credit_user_balances")+`
		SET active_remaining_amount = $2::decimal,
		    next_expires_at = (
		        SELECT MIN(expires_at)
		        FROM `+s.table("custom_gift_credit_grants")+`
		        WHERE user_id = $1
		          AND status = $3
		          AND remaining_amount > 0
		          AND expires_at IS NOT NULL
		          AND expires_at > $4
		    ),
		    refreshed_at = $4,
		    updated_at = $4
		WHERE user_id = $1
	`, userID, normalizeAmountDecimal(amount), types.StatusActive, now.UTC())
	if err != nil {
		return fmt.Errorf("set gift credit user balance: %w", err)
	}
	return nil
}

func (s *Store) recomputeUserBalance(ctx context.Context, exec SQLExecutor, userID int64, now time.Time) error {
	_, err := exec.ExecContext(ctx, `
		INSERT INTO `+s.table("custom_gift_credit_user_balances")+` (
			user_id, active_remaining_amount, next_expires_at, refreshed_at, updated_at
		)
		SELECT
			$1,
			COALESCE(SUM(remaining_amount), 0)::decimal,
			MIN(expires_at),
			$2,
			$2
		FROM `+s.table("custom_gift_credit_grants")+`
		WHERE user_id = $1
		  AND status = $3
		  AND remaining_amount > 0
		  AND (expires_at IS NULL OR expires_at > $2)
		ON CONFLICT (user_id) DO UPDATE SET
			active_remaining_amount = EXCLUDED.active_remaining_amount,
			next_expires_at = EXCLUDED.next_expires_at,
			refreshed_at = EXCLUDED.refreshed_at,
			updated_at = EXCLUDED.updated_at
	`, userID, now.UTC(), types.StatusActive)
	if err != nil {
		return fmt.Errorf("recompute gift credit user balance: %w", err)
	}
	return nil
}

func (s *Store) lockUsableGrants(ctx context.Context, exec SQLExecutor, userID int64, now time.Time, limit int) ([]types.Grant, error) {
	if limit <= 0 {
		return nil, types.ErrInvalidInput
	}
	rows, err := exec.QueryContext(ctx, `
		SELECT `+grantColumns()+`
		FROM `+s.table("custom_gift_credit_grants")+`
		WHERE user_id = $1
		  AND status = $2
		  AND remaining_amount > 0
		  AND (expires_at IS NULL OR expires_at > $3)
		ORDER BY expires_at ASC NULLS LAST, id ASC
		LIMIT $4
		FOR UPDATE
	`, userID, types.StatusActive, now.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("lock usable gift credit grants: %w", err)
	}
	defer rows.Close()
	return scanGrantRows(rows)
}

func (s *Store) applyGrantDeduction(ctx context.Context, exec SQLExecutor, grant types.Grant, input types.DeductInput, amount decimal.Decimal, now time.Time) (types.Deduction, error) {
	remainingAfter, err := parseAmount(grant.RemainingAmount)
	if err != nil {
		return types.Deduction{}, err
	}
	remainingAfter = remainingAfter.Sub(amount)
	status := types.StatusActive
	if remainingAfter.LessThanOrEqual(decimal.Zero) {
		status = types.StatusDepleted
		remainingAfter = decimal.Zero
	}
	result, err := exec.ExecContext(ctx, `
		UPDATE `+s.table("custom_gift_credit_grants")+`
		SET remaining_amount = $1::decimal,
		    status = $2,
		    updated_at = $3
		WHERE id = $4
		  AND user_id = $5
		  AND remaining_amount >= $6::decimal
		  AND status = $7
		  AND (expires_at IS NULL OR expires_at > $3)
	`, normalizeAmountDecimal(remainingAfter), status, now.UTC(), grant.ID, input.UserID,
		normalizeAmountDecimal(amount), types.StatusActive)
	if err != nil {
		return types.Deduction{}, fmt.Errorf("deduct gift credit grant: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return types.Deduction{}, fmt.Errorf("deduct gift credit grant: concurrent update")
	}
	row := exec.QueryRowContext(ctx, `
		INSERT INTO `+s.table("custom_gift_credit_deductions")+` (
			grant_id, user_id, request_id, usage_billing_key, amount, created_at
		) VALUES (
			$1, $2, $3, $4, $5::decimal, $6
		)
		RETURNING id, grant_id, user_id, request_id, usage_billing_key, amount::text, created_at
	`, grant.ID, input.UserID, strings.TrimSpace(input.RequestID), strings.TrimSpace(input.UsageBillingKey),
		normalizeAmountDecimal(amount), now.UTC())
	deduction, err := scanDeduction(row)
	if err != nil {
		return types.Deduction{}, fmt.Errorf("record gift credit deduction: %w", err)
	}
	return deduction, nil
}

func validateCreateGrantInput(input types.CreateGrantInput) error {
	if input.UserID <= 0 ||
		strings.TrimSpace(input.SourceType) == "" ||
		strings.TrimSpace(input.SourceID) == "" {
		return types.ErrInvalidInput
	}
	if _, err := parsePositiveAmount(input.Amount); err != nil {
		return types.ErrInvalidInput
	}
	createdAt := normalizeNow(input.CreatedAt)
	if input.ExpiresAt != nil && !input.ExpiresAt.After(createdAt) {
		return types.ErrInvalidInput
	}
	switch strings.TrimSpace(input.SourceType) {
	case types.SourceActivityReward, types.SourceAdminGrant, types.SourcePromoCode:
		return nil
	default:
		return types.ErrInvalidInput
	}
}

func (s *Store) table(name string) string {
	if s == nil {
		return name
	}
	return s.tablePrefix + name
}

func validateIdentifierPart(value string, allowEmpty bool) error {
	if strings.TrimSpace(value) == "" {
		if allowEmpty {
			return nil
		}
		return errors.New("identifier is required")
	}
	for _, r := range strings.TrimSpace(value) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return errors.New("identifier may only contain letters, digits or underscores")
	}
	return nil
}

func normalizeNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}
