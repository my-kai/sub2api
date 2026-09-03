package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/custom/modelratemultiplier/store"
	"github.com/Wei-Shaw/sub2api/internal/custom/modelratemultiplier/types"
)

// UserRateReader is the narrow main-service contract needed for priority one.
type UserRateReader interface {
	GetByUserAndGroup(ctx context.Context, userID, groupID int64) (*float64, error)
}

// Service validates, stores and resolves model-specific group multipliers.
type Service struct {
	store    *store.Store
	userRate UserRateReader
	mu       sync.RWMutex
	cache    map[int64]map[string]float64
}

// NewService creates a model-rate service.
func NewService(modelStore *store.Store, userRate UserRateReader) *Service {
	return &Service{store: modelStore, userRate: userRate, cache: make(map[int64]map[string]float64)}
}

// NormalizeModel enforces exact, case-insensitive model matching.
func NormalizeModel(model string) string { return strings.ToLower(strings.TrimSpace(model)) }

// ValidateEntries rejects malformed or ambiguous batch input before SQL writes.
func ValidateEntries(entries []types.Input) ([]types.Input, error) {
	result := make([]types.Input, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		model := NormalizeModel(entry.Model)
		if model == "" {
			return nil, errors.New("model must not be empty")
		}
		if _, ok := seen[model]; ok {
			return nil, fmt.Errorf("duplicate model %q", model)
		}
		if math.IsNaN(entry.RateMultiplier) || math.IsInf(entry.RateMultiplier, 0) || entry.RateMultiplier < 0 {
			return nil, fmt.Errorf("rate_multiplier for %q must be finite and >= 0", model)
		}
		seen[model] = struct{}{}
		result = append(result, types.Input{Model: model, RateMultiplier: entry.RateMultiplier})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Model < result[j].Model })
	return result, nil
}

// List returns stable model overrides for a group.
func (s *Service) List(ctx context.Context, groupID int64) ([]types.Entry, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("model rate multiplier service is not configured")
	}
	entries, err := s.store.ListByGroupID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	values := make(map[string]float64, len(entries))
	for _, entry := range entries {
		values[NormalizeModel(entry.Model)] = entry.RateMultiplier
	}
	s.mu.Lock()
	s.cache[groupID] = values
	s.mu.Unlock()
	return entries, nil
}

// Replace validates and atomically replaces a group's overrides.
func (s *Service) Replace(ctx context.Context, groupID int64, entries []types.Input) error {
	if groupID <= 0 {
		return errors.New("group id must be positive")
	}
	validated, err := ValidateEntries(entries)
	if err != nil {
		return err
	}
	if err := s.store.ReplaceByGroupID(ctx, groupID, validated); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, groupID)
	s.mu.Unlock()
	return nil
}

// Clear removes and invalidates all overrides for a group.
func (s *Service) Clear(ctx context.Context, groupID int64) error {
	if groupID <= 0 {
		return errors.New("group id must be positive")
	}
	if err := s.store.ClearByGroupID(ctx, groupID); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, groupID)
	s.mu.Unlock()
	return nil
}

// Resolve applies user-specific > model-specific > group-default priority.
func (s *Service) Resolve(ctx context.Context, userID, groupID int64, model string, groupDefault float64) float64 {
	if s == nil || s.store == nil || groupID <= 0 {
		return groupDefault
	}
	if s.userRate != nil && userID > 0 {
		userRate, err := s.userRate.GetByUserAndGroup(ctx, userID, groupID)
		if err != nil {
			slog.Warn("model_rate_user_override_load_failed", "user_id", userID, "group_id", groupID, "error", err)
		} else if userRate != nil {
			return *userRate
		}
	}
	key := NormalizeModel(model)
	if key == "" {
		return groupDefault
	}
	s.mu.RLock()
	values, ok := s.cache[groupID]
	rate, found := values[key]
	s.mu.RUnlock()
	if !ok {
		entries, err := s.List(ctx, groupID)
		if err != nil {
			slog.Warn("model_rate_override_load_failed", "group_id", groupID, "error", err)
			return groupDefault
		}
		for _, entry := range entries {
			if NormalizeModel(entry.Model) == key {
				return entry.RateMultiplier
			}
		}
		return groupDefault
	}
	if found {
		return rate
	}
	return groupDefault
}

// EntriesForGroups returns display-only entries keyed by group id.
func (s *Service) EntriesForGroups(ctx context.Context, groupIDs []int64) (map[int64][]types.Entry, error) {
	result := make(map[int64][]types.Entry, len(groupIDs))
	for _, groupID := range groupIDs {
		entries, err := s.List(ctx, groupID)
		if err != nil {
			return nil, err
		}
		result[groupID] = entries
	}
	return result, nil
}
