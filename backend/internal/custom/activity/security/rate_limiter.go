package security

import (
	"strings"
	"sync"
	"time"
)

// RateLimitRule defines a simple fixed-window limit.
type RateLimitRule struct {
	Limit  int
	Window time.Duration
}

type rateEntry struct {
	count     int
	expiresAt time.Time
}

// RateLimiter is an in-process guard for high-frequency activity abuse.
//
// Database-backed ticket/session replay checks remain the source of truth. This
// limiter only rejects obvious bursts before expensive crypto and SQL work.
type RateLimiter struct {
	now     func() time.Time
	mu      sync.Mutex
	entries map[string]rateEntry
}

// NewRateLimiter creates an in-memory fixed-window limiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		now:     func() time.Time { return time.Now().UTC() },
		entries: map[string]rateEntry{},
	}
}

// WithClock injects deterministic time for tests.
func (l *RateLimiter) WithClock(now func() time.Time) *RateLimiter {
	if now != nil {
		l.now = now
	}
	return l
}

// Allow records one event and returns false when the key exceeds its rule.
func (l *RateLimiter) Allow(scope string, key string, rule RateLimitRule) bool {
	if l == nil {
		return true
	}
	if rule.Limit <= 0 || rule.Window <= 0 {
		return true
	}
	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return false
	}
	now := l.now().UTC()
	entryKey := strings.TrimSpace(scope) + ":" + trimmedKey

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.entries[entryKey]
	if entry.expiresAt.IsZero() || !now.Before(entry.expiresAt) {
		entry = rateEntry{expiresAt: now.Add(rule.Window)}
	}
	entry.count++
	l.entries[entryKey] = entry
	return entry.count <= rule.Limit
}

// Sweep removes expired counters. Callers can use it opportunistically after requests.
func (l *RateLimiter) Sweep() {
	if l == nil {
		return
	}
	now := l.now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, entry := range l.entries {
		if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
			delete(l.entries, key)
		}
	}
}
