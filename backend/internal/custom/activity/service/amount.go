package service

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	zeroAmount = decimal.Zero
	oneAmount  = decimal.NewFromInt(1)
)

// parseAmount validates a decimal amount string used by activity money rules.
//
// Amounts are kept as decimal strings across the custom activity boundary so
// handlers and frontend code do not need to make fund decisions with float64.
func parseAmount(value string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return decimal.Zero, fmt.Errorf("amount is required")
	}
	amount, err := decimal.NewFromString(trimmed)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid amount %q: %w", value, err)
	}
	return amount, nil
}

// amountString normalizes all API-facing money values to the database scale.
func amountString(value decimal.Decimal) string {
	if value.IsNegative() {
		value = decimal.Zero
	}
	return value.Round(8).StringFixed(8)
}

func minAmount(first decimal.Decimal, rest ...decimal.Decimal) decimal.Decimal {
	result := first
	for _, item := range rest {
		if item.LessThan(result) {
			result = item
		}
	}
	return result
}

func positiveRemaining(cap decimal.Decimal, used decimal.Decimal) decimal.Decimal {
	remaining := cap.Sub(used)
	if remaining.IsNegative() {
		return decimal.Zero
	}
	return remaining
}
