package store

import (
	"database/sql"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/types"
	"github.com/shopspring/decimal"
)

type scanRow interface {
	Scan(dest ...any) error
}

func grantColumns() string {
	return `id, user_id, source_type, source_id, original_amount::text, remaining_amount::text,
		expires_at, status, note, created_by, created_at, updated_at`
}

func scanGrant(row scanRow) (types.Grant, error) {
	var grant types.Grant
	var createdBy sql.NullInt64
	var expiresAt sql.NullTime
	if err := row.Scan(
		&grant.ID,
		&grant.UserID,
		&grant.SourceType,
		&grant.SourceID,
		&grant.OriginalAmount,
		&grant.RemainingAmount,
		&expiresAt,
		&grant.Status,
		&grant.Note,
		&createdBy,
		&grant.CreatedAt,
		&grant.UpdatedAt,
	); err != nil {
		return types.Grant{}, err
	}
	if createdBy.Valid {
		grant.CreatedBy = &createdBy.Int64
	}
	if expiresAt.Valid {
		expiresAtUTC := expiresAt.Time.UTC()
		grant.ExpiresAt = &expiresAtUTC
	}
	grant.OriginalAmount = normalizeAmount(grant.OriginalAmount)
	grant.RemainingAmount = normalizeAmount(grant.RemainingAmount)
	return grant, nil
}

func scanGrantRows(rows *sql.Rows) ([]types.Grant, error) {
	result := []types.Grant{}
	for rows.Next() {
		grant, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, grant)
	}
	return result, rows.Err()
}

func scanUserBalance(row scanRow) (types.UserBalance, error) {
	var balance types.UserBalance
	var nextExpires sql.NullTime
	if err := row.Scan(
		&balance.UserID,
		&balance.ActiveRemainingAmount,
		&nextExpires,
		&balance.RefreshedAt,
		&balance.UpdatedAt,
	); err != nil {
		return types.UserBalance{}, err
	}
	if nextExpires.Valid {
		balance.NextExpiresAt = &nextExpires.Time
	}
	balance.ActiveRemainingAmount = normalizeAmount(balance.ActiveRemainingAmount)
	return balance, nil
}

func scanDeduction(row scanRow) (types.Deduction, error) {
	var deduction types.Deduction
	if err := row.Scan(
		&deduction.ID,
		&deduction.GrantID,
		&deduction.UserID,
		&deduction.RequestID,
		&deduction.UsageBillingKey,
		&deduction.Amount,
		&deduction.CreatedAt,
	); err != nil {
		return types.Deduction{}, err
	}
	deduction.Amount = normalizeAmount(deduction.Amount)
	return deduction, nil
}

func parseAmount(raw string) (decimal.Decimal, error) {
	return decimal.NewFromString(strings.TrimSpace(raw))
}

func parsePositiveAmount(raw string) (decimal.Decimal, error) {
	value, err := parseAmount(raw)
	if err != nil {
		return decimal.Zero, err
	}
	if value.LessThanOrEqual(decimal.Zero) {
		return decimal.Zero, types.ErrInvalidInput
	}
	return value, nil
}

func normalizeAmount(raw string) string {
	value, err := parseAmount(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return normalizeAmountDecimal(value)
}

func normalizeAmountDecimal(value decimal.Decimal) string {
	return value.Round(amountScale).StringFixed(amountScale)
}
