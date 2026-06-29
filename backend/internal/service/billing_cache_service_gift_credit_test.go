//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type billingEligibilityUserRepoStub struct {
	mockUserRepo
	balance float64
}

func (s *billingEligibilityUserRepoStub) GetByID(ctx context.Context, id int64) (*User, error) {
	return &User{ID: id, Balance: s.balance}, nil
}

type billingEligibilityGiftReaderStub struct {
	giftBalance   float64
	nextExpiresAt *time.Time
	err           error
}

func (s *billingEligibilityGiftReaderStub) GetUserGiftCreditBalance(ctx context.Context, userID int64) (GiftCreditBalance, error) {
	if s.err != nil {
		return GiftCreditBalance{}, s.err
	}
	return GiftCreditBalance{UserID: userID, GiftBalance: s.giftBalance, NextExpiresAt: s.nextExpiresAt}, nil
}

func TestBillingCacheServiceCheckBalanceEligibilityUsesGiftCredit(t *testing.T) {
	tests := []struct {
		name        string
		balance     float64
		giftBalance float64
		wantErr     error
	}{
		{
			name:        "no balance and no gift balance",
			balance:     0,
			giftBalance: 0,
			wantErr:     ErrInsufficientBalance,
		},
		{
			name:        "has balance and no gift balance",
			balance:     1,
			giftBalance: 0,
		},
		{
			name:        "no balance and has gift balance",
			balance:     0,
			giftBalance: 1,
		},
		{
			name:        "has balance and has gift balance",
			balance:     1,
			giftBalance: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewBillingCacheService(nil, &billingEligibilityUserRepoStub{balance: tt.balance}, nil, nil, nil, nil, &config.Config{}, nil)
			t.Cleanup(svc.Stop)
			svc.SetGiftCreditBalanceReader(&billingEligibilityGiftReaderStub{giftBalance: tt.giftBalance})

			err := svc.checkBalanceEligibility(context.Background(), 42)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBillingCacheServiceCheckBalanceEligibilityDoesNotFallbackWhenGiftReadFails(t *testing.T) {
	svc := NewBillingCacheService(nil, &billingEligibilityUserRepoStub{balance: 1}, nil, nil, nil, nil, &config.Config{}, nil)
	t.Cleanup(svc.Stop)
	svc.SetGiftCreditBalanceReader(&billingEligibilityGiftReaderStub{err: errors.New("gift aggregate unavailable")})

	require.Error(t, svc.checkBalanceEligibility(context.Background(), 42))
}
