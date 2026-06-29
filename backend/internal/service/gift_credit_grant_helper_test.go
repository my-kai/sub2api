//go:build unit

package service

import (
	"testing"
	"time"

	gifttypes "github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/types"
	"github.com/stretchr/testify/require"
)

func TestValidateGiftCreditGrantInputRequiresSourceID(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	err := validateGiftCreditGrantInput(gifttypes.CreateGrantInput{
		UserID:     7,
		SourceType: gifttypes.SourcePromoCode,
		Amount:     "1.00000000",
		ExpiresAt:  &expiresAt,
		CreatedAt:  time.Now().UTC(),
	})

	require.ErrorIs(t, err, gifttypes.ErrInvalidInput)
}

func TestValidateGiftCreditGrantInputAcceptsExplicitSourceID(t *testing.T) {
	now := time.Now().UTC()
	err := validateGiftCreditGrantInput(gifttypes.CreateGrantInput{
		UserID:     7,
		SourceType: gifttypes.SourcePromoCode,
		SourceID:   "promo:1:usage:2",
		Amount:     "1.00000000",
		ExpiresAt:  ptrGiftGrantTime(now.Add(time.Hour)),
		CreatedAt:  now,
	})

	require.NoError(t, err)
}

func TestValidateGiftCreditGrantInputRejectsNonPositiveAmount(t *testing.T) {
	now := time.Now().UTC()
	err := validateGiftCreditGrantInput(gifttypes.CreateGrantInput{
		UserID:     7,
		SourceType: gifttypes.SourcePromoCode,
		SourceID:   "promo:1:usage:2",
		Amount:     "0.00000000",
		ExpiresAt:  ptrGiftGrantTime(now.Add(time.Hour)),
		CreatedAt:  now,
	})

	require.ErrorIs(t, err, gifttypes.ErrInvalidInput)
}

func TestValidateGiftCreditGrantInputAcceptsPermanentExpiry(t *testing.T) {
	now := time.Now().UTC()
	err := validateGiftCreditGrantInput(gifttypes.CreateGrantInput{
		UserID:     7,
		SourceType: gifttypes.SourcePromoCode,
		SourceID:   "promo:1:usage:2",
		Amount:     "1.00000000",
		CreatedAt:  now,
	})

	require.NoError(t, err)
}

func ptrGiftGrantTime(value time.Time) *time.Time {
	return &value
}
