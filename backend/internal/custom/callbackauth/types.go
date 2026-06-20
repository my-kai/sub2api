package callbackauth

import (
	"errors"
	"time"
)

const (
	defaultCodeTTL = 5 * time.Minute
	minCodeTTL     = time.Minute
	maxCodeTTL     = 10 * time.Minute
	codeByteLength = 32
	codePrefix     = "cb_"
)

var (
	ErrInvalidCallback = errors.New("callback url invalid")
	ErrCallbackDenied  = errors.New("callback domain denied")
	ErrCodeExpired     = errors.New("callback auth code expired")
)

// CallbackInfo is returned to the browser authorization page before the user
// confirms consent for the callback domain.
type CallbackInfo struct {
	Callback   string `json:"callback"`
	Domain     string `json:"domain"`
	Authorized bool   `json:"authorized"`
}

// AuthorizeResponse contains the redirect target after consent is accepted.
type AuthorizeResponse struct {
	RedirectURL string    `json:"redirect_url"`
	Code        string    `json:"code"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// UserSnapshot is the small identity payload exposed through code exchange.
// It intentionally excludes tokens, password hashes, balance, quota and groups.
type UserSnapshot struct {
	ID             int64     `json:"id"`
	ExternalUserID string    `json:"external_user_id"`
	Username       string    `json:"username,omitempty"`
	Email          string    `json:"email,omitempty"`
	Role           string    `json:"role,omitempty"`
	IsAdmin        bool      `json:"is_admin"`
	IssuedAt       time.Time `json:"issued_at"`
}

// ExchangeResponse is returned to the callback system after a one-time code is
// consumed successfully.
type ExchangeResponse struct {
	User           UserSnapshot `json:"user"`
	CallbackDomain string       `json:"callback_domain"`
	CallbackURL    string       `json:"callback_url"`
	ExpiresAt      time.Time    `json:"expires_at"`
}

type callbackTarget struct {
	URL    string
	Domain string
}

type codeRecord struct {
	UserID         int64
	CallbackURL    string
	CallbackDomain string
	ExpiresAt      time.Time
	CreatedAt      time.Time
}
