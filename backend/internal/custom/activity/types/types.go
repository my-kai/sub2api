package types

import (
	"errors"
	"time"
)

const (
	// ActivityTypeRedPacketRain is the first custom activity implemented by this module.
	ActivityTypeRedPacketRain ActivityType = "red_packet_rain"

	// ActivityStatusDraft keeps an activity hidden while admins prepare it.
	ActivityStatusDraft ActivityStatus = "draft"
	// ActivityStatusScheduled marks an activity visible to admins and waiting for its start time.
	ActivityStatusScheduled ActivityStatus = "scheduled"
	// ActivityStatusActive marks an activity currently open for user interaction.
	ActivityStatusActive ActivityStatus = "active"
	// ActivityStatusEnded marks an activity that finished normally or was ended early.
	ActivityStatusEnded ActivityStatus = "ended"
	// ActivityStatusOffline marks an activity removed from user-facing lists.
	ActivityStatusOffline ActivityStatus = "offline"

	// RoundStatusWaiting means the activity is live but this round has not started.
	RoundStatusWaiting RoundStatus = "waiting"
	// RoundStatusActive means users may submit hit counts for this round.
	RoundStatusActive RoundStatus = "active"
	// RoundStatusEnded means this single round is closed.
	RoundStatusEnded RoundStatus = "ended"
	// RoundStatusFinished means all rounds in the activity are closed.
	RoundStatusFinished RoundStatus = "finished"
	// RoundStatusOffline mirrors an offline activity so clients can stop polling.
	RoundStatusOffline RoundStatus = "offline"
)

var (
	// ErrNotFound is returned when a custom activity record is missing.
	ErrNotFound = errors.New("custom activity not found")
	// ErrInvalidInput is returned before SQL execution when required fields are unusable.
	ErrInvalidInput = errors.New("custom activity input is invalid")
	// ErrActivityNotStarted is returned when users claim before the activity window opens.
	ErrActivityNotStarted = errors.New("custom activity is not started")
	// ErrActivityEnded is returned when users claim after the activity window closes.
	ErrActivityEnded = errors.New("custom activity is ended")
	// ErrActivityOffline is returned when an activity has been removed from user-facing pages.
	ErrActivityOffline = errors.New("custom activity is offline")
	// ErrRoundNotStarted is returned when a submitted round is still waiting.
	ErrRoundNotStarted = errors.New("custom activity round is not started")
	// ErrRoundEnded is returned when a submitted round is no longer claimable.
	ErrRoundEnded = errors.New("custom activity round is ended")
	// ErrUserRoundCapReached is returned when the user has no remaining quota in the round.
	ErrUserRoundCapReached = errors.New("custom activity round cap reached")
	// ErrUserTotalCapReached is returned when the user has no remaining quota for the activity.
	ErrUserTotalCapReached = errors.New("custom activity user cap reached")
	// ErrBudgetExhausted is returned when the activity has no remaining reward budget.
	ErrBudgetExhausted = errors.New("custom activity budget exhausted")
	// ErrRedPacketRainSecurityRejected is returned when WebSocket claim security checks fail.
	ErrRedPacketRainSecurityRejected = errors.New("custom activity red packet rain security rejected")
)

// ActivityType identifies the concrete activity implementation.
type ActivityType string

// ActivityStatus is the persisted lifecycle for activity cards and details.
type ActivityStatus string

// RoundStatus is the persisted lifecycle for a red packet rain round.
type RoundStatus string

// Activity stores the common fields shared by all custom activities.
//
// The first release only accepts ActivityTypeRedPacketRain, but the common
// envelope keeps the activity hall independent from red packet rain internals.
type Activity struct {
	ID          int64          `json:"id"`
	Type        ActivityType   `json:"type"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	CoverURL    string         `json:"cover_url"`
	Status      ActivityStatus `json:"status"`
	StartsAt    time.Time      `json:"starts_at"`
	EndsAt      time.Time      `json:"ends_at"`
	CreatedBy   int64          `json:"created_by,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	EndedAt     *time.Time     `json:"ended_at,omitempty"`
}

// RedPacketRainConfig stores the admin-controlled money and round rules.
//
// Amount fields intentionally stay as decimal strings in Go DTOs so handler and
// frontend layers cannot accidentally make funds decisions with float64.
type RedPacketRainConfig struct {
	ActivityID           int64     `json:"activity_id"`
	RoundCount           int       `json:"round_count"`
	RoundDurationSeconds int       `json:"round_duration_seconds"`
	RoundIntervalSeconds int       `json:"round_interval_seconds"`
	TotalBudget          string    `json:"total_budget"`
	PerUserRoundCap      string    `json:"per_user_round_cap"`
	PerUserTotalCap      string    `json:"per_user_total_cap"`
	BaseUnitAmount       string    `json:"base_unit_amount"`
	MaxSingleReward      string    `json:"max_single_reward"`
	ProbabilityStep      string    `json:"probability_step"`
	GiftValidityDays     int       `json:"gift_validity_days"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// RedPacketRainRound stores an auditable pre-generated round window.
type RedPacketRainRound struct {
	ID         int64       `json:"id"`
	ActivityID int64       `json:"activity_id"`
	RoundNo    int         `json:"round_no"`
	StartsAt   time.Time   `json:"starts_at"`
	EndsAt     time.Time   `json:"ends_at"`
	Status     RoundStatus `json:"status"`
	CreatedAt  time.Time   `json:"created_at"`
}

// RedPacketRainClaim records one idempotent settlement attempt.
//
// Claims with HitCount equal to zero are still useful audit records, but their
// RewardAmount must remain zero and later service code must skip balance writes.
type RedPacketRainClaim struct {
	ID             int64     `json:"id"`
	ActivityID     int64     `json:"activity_id"`
	RoundID        int64     `json:"round_id"`
	UserID         int64     `json:"user_id"`
	HitCount       int       `json:"hit_count"`
	RewardAmount   string    `json:"reward_amount"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// RedPacketRainWSTicket persists a one-time WebSocket entry ticket.
//
// Only TicketHash is stored. The raw ticket is returned once to the browser and
// cannot be reconstructed from the database if audit rows are leaked.
type RedPacketRainWSTicket struct {
	ID                int64      `json:"id"`
	TicketHash        string     `json:"ticket_hash"`
	ActivityID        int64      `json:"activity_id"`
	RoundID           int64      `json:"round_id"`
	UserID            int64      `json:"user_id"`
	DeviceFingerprint string     `json:"device_fingerprint"`
	ClientNonce       string     `json:"client_nonce"`
	ExpiresAt         time.Time  `json:"expires_at"`
	ConsumedAt        *time.Time `json:"consumed_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// RedPacketRainWSSession stores the server-owned state for one claim socket.
//
// UsedNonces is intentionally a string slice so store code can keep replay
// protection simple without exposing JSON implementation details to callers.
type RedPacketRainWSSession struct {
	ID                int64      `json:"id"`
	SessionID         string     `json:"session_id"`
	ActivityID        int64      `json:"activity_id"`
	RoundID           int64      `json:"round_id"`
	UserID            int64      `json:"user_id"`
	DeviceFingerprint string     `json:"device_fingerprint"`
	ClientNonce       string     `json:"client_nonce"`
	ServerNonce       string     `json:"server_nonce"`
	ChallengeHash     string     `json:"challenge_hash"`
	UsedNonces        []string   `json:"used_nonces"`
	RiskStatus        string     `json:"risk_status"`
	RiskReason        string     `json:"risk_reason"`
	ExpiresAt         time.Time  `json:"expires_at"`
	CreatedAt         time.Time  `json:"created_at"`
	ClosedAt          *time.Time `json:"closed_at,omitempty"`
}

// ClaimSummary captures the totals needed by the later settlement service.
type ClaimSummary struct {
	ActivityIssuedAmount string `json:"activity_issued_amount"`
	UserRoundAmount      string `json:"user_round_amount"`
	UserActivityAmount   string `json:"user_activity_amount"`
	ParticipantCount     int64  `json:"participant_count"`
}

// UserRewardState describes user-side cap progress after a state query or claim.
type UserRewardState struct {
	RoundTotal         string `json:"round_total"`
	ActivityTotal      string `json:"activity_total"`
	RoundRemaining     string `json:"round_remaining"`
	ActivityRemaining  string `json:"activity_remaining"`
	RoundCapReached    bool   `json:"round_cap_reached"`
	ActivityCapReached bool   `json:"activity_cap_reached"`
}

// BudgetState describes activity-level reward budget progress.
type BudgetState struct {
	Remaining string `json:"remaining"`
	Exhausted bool   `json:"exhausted"`
}

// ClaimResult is the service-level settlement result used by later handlers.
type ClaimResult struct {
	ClaimID      int64           `json:"claim_id"`
	ActivityID   int64           `json:"activity_id"`
	RoundID      int64           `json:"round_id"`
	HitCount     int             `json:"hit_count"`
	RewardAmount string          `json:"reward_amount"`
	Credited     bool            `json:"credited"`
	Duplicate    bool            `json:"duplicate"`
	Message      string          `json:"message"`
	UserReward   UserRewardState `json:"user_reward"`
	Budget       BudgetState     `json:"budget"`
}

// RedPacketRainRoundState is the current or next round state returned to clients.
type RedPacketRainRoundState struct {
	ID                int64       `json:"id"`
	RoundNo           int         `json:"round_no"`
	Status            RoundStatus `json:"status"`
	StartsAt          time.Time   `json:"starts_at"`
	EndsAt            time.Time   `json:"ends_at"`
	ServerNow         time.Time   `json:"server_now"`
	SecondsUntilStart int64       `json:"seconds_until_start"`
	SecondsUntilEnd   int64       `json:"seconds_until_end"`
}

// RedPacketRainState is the pollable state for a user's activity detail page.
type RedPacketRainState struct {
	ActivityID  int64                    `json:"activity_id"`
	Status      RoundStatus              `json:"status"`
	Round       *RedPacketRainRoundState `json:"round,omitempty"`
	UserReward  UserRewardState          `json:"user_reward"`
	Budget      BudgetState              `json:"budget"`
	ServerNow   time.Time                `json:"server_now"`
	FinishedAll bool                     `json:"finished_all"`
}

// ActivityAdminSummary is the compact aggregate used by the admin list.
type ActivityAdminSummary struct {
	Activity         Activity `json:"activity"`
	TotalBudget      string   `json:"total_budget"`
	IssuedAmount     string   `json:"issued_amount"`
	ParticipantCount int64    `json:"participant_count"`
}

// PageRequest keeps store pagination consistent before handlers are added.
type PageRequest struct {
	Page     int
	PageSize int
}
