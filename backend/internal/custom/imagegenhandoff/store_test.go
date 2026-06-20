package imagegenhandoff

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryCodeStoreConsumeOnce(t *testing.T) {
	store := NewMemoryCodeStore(5 * time.Minute)
	record, err := store.Create(context.Background(), Identity{ExternalUserID: "42", IssuedAt: time.Now()})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	consumed, err := store.Consume(context.Background(), record.Code)
	if err != nil {
		t.Fatalf("first Consume() error = %v", err)
	}
	if consumed.Identity.ExternalUserID != "42" {
		t.Fatalf("ExternalUserID = %q, want 42", consumed.Identity.ExternalUserID)
	}

	if _, err := store.Consume(context.Background(), record.Code); !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("second Consume() error = %v, want ErrCodeExpired", err)
	}
}

func TestMemoryCodeStoreExpiredCodeDeleted(t *testing.T) {
	store := NewMemoryCodeStore(time.Minute)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	record, err := store.Create(context.Background(), Identity{ExternalUserID: "42", IssuedAt: now})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	now = now.Add(2 * time.Minute)
	if _, err := store.Consume(context.Background(), record.Code); !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("Consume(expired) error = %v, want ErrCodeExpired", err)
	}
	if _, exists := store.records[record.Code]; exists {
		t.Fatalf("expired code should be removed from memory")
	}
}
