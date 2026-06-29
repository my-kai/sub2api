package types

import (
	"errors"
	"time"
)

const (
	// SourceActivityReward identifies red-packet-rain or future activity rewards.
	SourceActivityReward = "activity_reward"
	// SourceAdminGrant identifies manually granted gift credit from admins.
	SourceAdminGrant = "admin_grant"
	// SourcePromoCode identifies promo-code redemption gift credit.
	SourcePromoCode = "promo_code"

	// StatusActive means a grant may still be used before its expiry time.
	StatusActive = "active"
	// StatusDepleted means the grant has been fully consumed.
	StatusDepleted = "depleted"
	// StatusExpired means remaining credit can no longer be consumed.
	StatusExpired = "expired"
)

var (
	// ErrInvalidInput is returned before persistence when a request is unusable.
	ErrInvalidInput = errors.New("gift credit input is invalid")
	// ErrInsufficientGiftCredit is returned only by gift-only operations.
	ErrInsufficientGiftCredit = errors.New("insufficient gift credit")
)

// Grant records one independently expiring gift-credit allocation.
type Grant struct {
	ID              int64      `json:"id"`
	UserID          int64      `json:"user_id"`
	SourceType      string     `json:"source_type"`
	SourceID        string     `json:"source_id"`
	OriginalAmount  string     `json:"original_amount"`
	RemainingAmount string     `json:"remaining_amount"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	Status          string     `json:"status"`
	Note            string     `json:"note"`
	CreatedBy       *int64     `json:"created_by,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// UserBalance is the O(1) aggregate used by AI request eligibility checks.
type UserBalance struct {
	UserID                int64      `json:"user_id"`
	ActiveRemainingAmount string     `json:"active_remaining_amount"`
	NextExpiresAt         *time.Time `json:"next_expires_at,omitempty"`
	RefreshedAt           time.Time  `json:"refreshed_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// Deduction records how much one request consumed from a grant.
type Deduction struct {
	ID              int64     `json:"id"`
	GrantID         int64     `json:"grant_id"`
	UserID          int64     `json:"user_id"`
	RequestID       string    `json:"request_id"`
	UsageBillingKey string    `json:"usage_billing_key"`
	Amount          string    `json:"amount"`
	CreatedAt       time.Time `json:"created_at"`
}

// CreateGrantInput is the validated shape for new gift-credit grants.
type CreateGrantInput struct {
	UserID     int64
	SourceType string
	SourceID   string
	Amount     string
	ExpiresAt  *time.Time
	Note       string
	CreatedBy  *int64
	CreatedAt  time.Time
}

// DeductInput is the request-scoped front deduction before normal balance logic.
type DeductInput struct {
	UserID          int64
	Amount          string
	RequestID       string
	UsageBillingKey string
	Now             time.Time
}

// DeductResult tells callers how much gift credit was used and what remains for normal balance.
type DeductResult struct {
	GiftDeducted       string
	RemainingCost      string
	NewGiftBalance     string
	Deductions         []Deduction
	SkippedGrantLookup bool
}
