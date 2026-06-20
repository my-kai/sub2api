package security

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRateLimiterFixedWindow(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	limiter := NewRateLimiter().WithClock(func() time.Time { return now })
	rule := RateLimitRule{Limit: 2, Window: time.Second}

	require.True(t, limiter.Allow("ticket", "user-1", rule))
	require.True(t, limiter.Allow("ticket", "user-1", rule))
	require.False(t, limiter.Allow("ticket", "user-1", rule))

	now = now.Add(time.Second)
	require.True(t, limiter.Allow("ticket", "user-1", rule))
}
