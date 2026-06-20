package imagegenhandoff

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	defaultCodeTTL = 5 * time.Minute
	minCodeTTL     = time.Minute
	maxCodeTTL     = 10 * time.Minute
	codeByteLength = 32
	codePrefix     = "once_"
)

var (
	ErrInvalidConfig = errors.New("image-gen handoff config invalid")
	ErrCodeExpired   = errors.New("image-gen login code expired")
)

// Identity is the small user snapshot transferred to image-gen through code exchange.
// It intentionally excludes auth tokens, password fields, balance, and permission lists.
type Identity struct {
	ExternalUserID string    `json:"external_user_id"`
	Username       string    `json:"username,omitempty"`
	Email          string    `json:"email,omitempty"`
	Role           string    `json:"role,omitempty"`
	IsAdmin        bool      `json:"is_admin"`
	IssuedAt       time.Time `json:"issued_at"`
}

// CodeRecord stores the one-time handoff payload until image-gen consumes it.
type CodeRecord struct {
	Code      string
	Identity  Identity
	ExpiresAt time.Time
}

// CodeStore defines one-time code persistence so a future Redis implementation
// can replace the in-memory first-phase store without changing handlers.
type CodeStore interface {
	Create(ctx context.Context, identity Identity) (CodeRecord, error)
	Consume(ctx context.Context, code string) (CodeRecord, error)
}

// MemoryCodeStore keeps handoff codes in process memory.
// Note: this first-phase implementation only supports a single sub2api-ex instance;
// multi-instance deployments should replace it with Redis or PostgreSQL.
type MemoryCodeStore struct {
	now func() time.Time
	ttl time.Duration

	mu      sync.Mutex
	records map[string]CodeRecord
}

// NewMemoryCodeStore creates a one-time code store with bounded TTL.
func NewMemoryCodeStore(ttl time.Duration) *MemoryCodeStore {
	return &MemoryCodeStore{
		now:     time.Now,
		ttl:     normalizeTTL(ttl),
		records: make(map[string]CodeRecord),
	}
}

// Create generates a high-entropy code and stores the minimal identity snapshot.
func (s *MemoryCodeStore) Create(_ context.Context, identity Identity) (CodeRecord, error) {
	if strings.TrimSpace(identity.ExternalUserID) == "" {
		return CodeRecord{}, ErrInvalidConfig
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked(s.now())
	for {
		code, err := randomCode()
		if err != nil {
			return CodeRecord{}, err
		}
		if _, exists := s.records[code]; exists {
			continue
		}

		record := CodeRecord{
			Code:      code,
			Identity:  identity,
			ExpiresAt: s.now().Add(s.ttl).UTC(),
		}
		s.records[code] = record
		return record, nil
	}
}

// Consume returns and deletes a code. Expired codes are also deleted so replay
// attempts cannot distinguish expired from already-consumed states.
func (s *MemoryCodeStore) Consume(_ context.Context, code string) (CodeRecord, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return CodeRecord{}, ErrCodeExpired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.cleanupLocked(now)
	record, exists := s.records[code]
	if !exists {
		return CodeRecord{}, ErrCodeExpired
	}
	delete(s.records, code)
	if !record.ExpiresAt.After(now) {
		return CodeRecord{}, ErrCodeExpired
	}
	return record, nil
}

func normalizeTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultCodeTTL
	}
	if ttl < minCodeTTL {
		return minCodeTTL
	}
	if ttl > maxCodeTTL {
		return maxCodeTTL
	}
	return ttl
}

func (s *MemoryCodeStore) cleanupLocked(now time.Time) {
	for code, record := range s.records {
		if !record.ExpiresAt.After(now) {
			delete(s.records, code)
		}
	}
}

func randomCode() (string, error) {
	raw := make([]byte, codeByteLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return codePrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}
