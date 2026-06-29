package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/store"
	"github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/types"
	mainservice "github.com/Wei-Shaw/sub2api/internal/service"
)

// Service exposes gift-credit operations to thin main-repo integration points.
type Service struct {
	store *store.Store
	now   func() time.Time
}

// NewService creates a gift-credit service around the custom store.
func NewService(giftStore *store.Store) *Service {
	return &Service{
		store: giftStore,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithClock injects a deterministic clock for tests.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// GetUserGiftCreditBalance returns the aggregate gift balance for O(1) callers.
//
// It reads custom_gift_credit_user_balances and lets the store perform a
// lightweight per-user refresh when the next known grant has expired.
func (s *Service) GetUserGiftCreditBalance(ctx context.Context, userID int64) (mainservice.GiftCreditBalance, error) {
	if s == nil || s.store == nil {
		return mainservice.GiftCreditBalance{}, fmt.Errorf("gift credit service is not configured")
	}
	balance, err := s.store.UserBalance(ctx, userID, s.now())
	if err != nil {
		return mainservice.GiftCreditBalance{}, err
	}
	amount, err := strconv.ParseFloat(balance.ActiveRemainingAmount, 64)
	if err != nil {
		return mainservice.GiftCreditBalance{}, fmt.Errorf("parse gift credit balance: %w", err)
	}
	return mainservice.GiftCreditBalance{
		UserID:        userID,
		GiftBalance:   amount,
		NextExpiresAt: balance.NextExpiresAt,
	}, nil
}

// GetUsersGiftCreditBalance returns aggregate gift balances for user-list pages.
func (s *Service) GetUsersGiftCreditBalance(ctx context.Context, userIDs []int64) (map[int64]mainservice.GiftCreditBalance, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("gift credit service is not configured")
	}
	balances, err := s.store.UserBalances(ctx, userIDs, s.now())
	if err != nil {
		return nil, err
	}
	result := make(map[int64]mainservice.GiftCreditBalance, len(balances))
	for userID, balance := range balances {
		amount, err := strconv.ParseFloat(balance.ActiveRemainingAmount, 64)
		if err != nil {
			return nil, fmt.Errorf("parse gift credit balance: %w", err)
		}
		result[userID] = mainservice.GiftCreditBalance{
			UserID:        userID,
			GiftBalance:   amount,
			NextExpiresAt: balance.NextExpiresAt,
		}
	}
	return result, nil
}

// CreateGrant delegates grant creation to the custom store.
func (s *Service) CreateGrant(ctx context.Context, input types.CreateGrantInput) (types.Grant, error) {
	if s == nil || s.store == nil {
		return types.Grant{}, fmt.Errorf("gift credit service is not configured")
	}
	return s.store.CreateGrant(ctx, input)
}
