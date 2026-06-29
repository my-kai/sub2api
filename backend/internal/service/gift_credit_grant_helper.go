package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	gifttypes "github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/types"
	"github.com/shopspring/decimal"
)

type giftCreditSQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// createGiftCreditGrantSQL creates a grant and refreshes the aggregate in one caller-owned transaction.
//
// The helper exists for main service flows that already run inside an ent
// transaction. The custom giftcredit.Store owns the same SQL, but its public
// CreateGrant starts its own transaction and therefore cannot be used inside
// promo-code redemption without losing atomicity between usage and grant rows.
func createGiftCreditGrantSQL(ctx context.Context, exec giftCreditSQLExecutor, input gifttypes.CreateGrantInput) (gifttypes.Grant, error) {
	if exec == nil {
		return gifttypes.Grant{}, errors.New("gift credit SQL executor is not configured")
	}
	if err := validateGiftCreditGrantInput(input); err != nil {
		return gifttypes.Grant{}, err
	}
	grantsTable, err := giftCreditTableName("custom_gift_credit_grants")
	if err != nil {
		return gifttypes.Grant{}, err
	}
	balancesTable, err := giftCreditTableName("custom_gift_credit_user_balances")
	if err != nil {
		return gifttypes.Grant{}, err
	}

	createdAt := normalizeGiftCreatedAt(input.CreatedAt)
	amount := strings.TrimSpace(input.Amount)
	var expiresAt any
	if input.ExpiresAt != nil {
		expiresAt = input.ExpiresAt.UTC()
	}
	rows, err := exec.QueryContext(ctx, `
		INSERT INTO `+grantsTable+` (
			user_id, source_type, source_id, original_amount, remaining_amount,
			expires_at, status, note, created_by, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4::decimal, $4::decimal, $5, $6, $7, $8, $9, $9
		)
		RETURNING id, user_id, source_type, source_id, original_amount::text, remaining_amount::text,
		          expires_at, status, note, created_by, created_at, updated_at
	`, input.UserID, strings.TrimSpace(input.SourceType), strings.TrimSpace(input.SourceID),
		amount, expiresAt, gifttypes.StatusActive, strings.TrimSpace(input.Note),
		input.CreatedBy, createdAt)
	if err != nil {
		return gifttypes.Grant{}, fmt.Errorf("create gift credit grant: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return gifttypes.Grant{}, fmt.Errorf("create gift credit grant: %w", err)
		}
		return gifttypes.Grant{}, fmt.Errorf("create gift credit grant: no row returned")
	}
	grant, err := scanGiftCreditGrant(rows)
	if err != nil {
		return gifttypes.Grant{}, err
	}
	if rows.Next() {
		return gifttypes.Grant{}, fmt.Errorf("create gift credit grant: multiple rows returned")
	}
	if err := rows.Err(); err != nil {
		return gifttypes.Grant{}, fmt.Errorf("create gift credit grant: %w", err)
	}

	// The aggregate is the O(1) source for request admission and balance display.
	if _, err := exec.ExecContext(ctx, `
		INSERT INTO `+balancesTable+` (
			user_id, active_remaining_amount, next_expires_at, refreshed_at, updated_at
		) VALUES (
			$1, $2::decimal, $3, $4, $4
		)
		ON CONFLICT (user_id) DO UPDATE SET
			active_remaining_amount = `+balancesTable+`.active_remaining_amount + EXCLUDED.active_remaining_amount,
			next_expires_at = CASE
				WHEN `+balancesTable+`.next_expires_at IS NULL THEN EXCLUDED.next_expires_at
				WHEN EXCLUDED.next_expires_at IS NULL THEN `+balancesTable+`.next_expires_at
				WHEN EXCLUDED.next_expires_at < `+balancesTable+`.next_expires_at THEN EXCLUDED.next_expires_at
				ELSE `+balancesTable+`.next_expires_at
			END,
			updated_at = EXCLUDED.updated_at
	`, input.UserID, amount, expiresAt, createdAt); err != nil {
		return gifttypes.Grant{}, fmt.Errorf("upsert gift credit user balance: %w", err)
	}
	return grant, nil
}

func validateGiftCreditGrantInput(input gifttypes.CreateGrantInput) error {
	if input.UserID <= 0 ||
		strings.TrimSpace(input.SourceType) == "" ||
		strings.TrimSpace(input.SourceID) == "" ||
		strings.TrimSpace(input.Amount) == "" {
		return gifttypes.ErrInvalidInput
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(input.Amount))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return gifttypes.ErrInvalidInput
	}
	createdAt := normalizeGiftCreatedAt(input.CreatedAt)
	if input.ExpiresAt != nil && !input.ExpiresAt.After(createdAt) {
		return gifttypes.ErrInvalidInput
	}
	switch strings.TrimSpace(input.SourceType) {
	case gifttypes.SourceActivityReward, gifttypes.SourceAdminGrant, gifttypes.SourcePromoCode:
		return nil
	default:
		return gifttypes.ErrInvalidInput
	}
}

func scanGiftCreditGrant(rows *sql.Rows) (gifttypes.Grant, error) {
	var grant gifttypes.Grant
	var expiresAt sql.NullTime
	if err := rows.Scan(
		&grant.ID,
		&grant.UserID,
		&grant.SourceType,
		&grant.SourceID,
		&grant.OriginalAmount,
		&grant.RemainingAmount,
		&expiresAt,
		&grant.Status,
		&grant.Note,
		&grant.CreatedBy,
		&grant.CreatedAt,
		&grant.UpdatedAt,
	); err != nil {
		return gifttypes.Grant{}, fmt.Errorf("scan gift credit grant: %w", err)
	}
	if expiresAt.Valid {
		expiresAtUTC := expiresAt.Time.UTC()
		grant.ExpiresAt = &expiresAtUTC
	}
	grant.CreatedAt = grant.CreatedAt.UTC()
	grant.UpdatedAt = grant.UpdatedAt.UTC()
	return grant, nil
}

func normalizeGiftCreatedAt(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func giftCreditTableName(name string) (string, error) {
	prefix := strings.TrimSpace(os.Getenv("CUSTOM_GIFT_CREDIT_TABLE_PREFIX"))
	if err := validateSQLIdentifierPart(prefix, true); err != nil {
		return "", fmt.Errorf("gift credit table prefix is invalid: %w", err)
	}
	if err := validateSQLIdentifierPart(name, false); err != nil {
		return "", fmt.Errorf("gift credit table name is invalid: %w", err)
	}
	return prefix + name, nil
}

func validateSQLIdentifierPart(value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return errors.New("identifier is required")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return errors.New("identifier may only contain letters, digits or underscores")
	}
	return nil
}
