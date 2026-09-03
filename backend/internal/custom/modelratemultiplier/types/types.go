package types

import "time"

// Entry is one exact model override belonging to a group.
type Entry struct {
	ID             int64     `json:"id"`
	GroupID        int64     `json:"group_id"`
	Model          string    `json:"model"`
	RateMultiplier float64   `json:"rate_multiplier"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Input is the writable portion of an Entry.
type Input struct {
	Model          string  `json:"model"`
	RateMultiplier float64 `json:"rate_multiplier"`
}
