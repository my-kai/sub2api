//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type promoRepoStubForGiftCredit struct {
	created *PromoCode
	updated *PromoCode
	byID    *PromoCode
	byCode  *PromoCode
}

func (s *promoRepoStubForGiftCredit) Create(ctx context.Context, code *PromoCode) error {
	clone := *code
	s.created = &clone
	return nil
}

func (s *promoRepoStubForGiftCredit) GetByID(ctx context.Context, id int64) (*PromoCode, error) {
	if s.byID == nil {
		panic("unexpected GetByID call")
	}
	clone := *s.byID
	return &clone, nil
}

func (s *promoRepoStubForGiftCredit) GetByCode(ctx context.Context, code string) (*PromoCode, error) {
	if s.byCode == nil {
		panic("unexpected GetByCode call")
	}
	clone := *s.byCode
	return &clone, nil
}

func (s *promoRepoStubForGiftCredit) GetByCodeForUpdate(ctx context.Context, code string) (*PromoCode, error) {
	panic("unexpected GetByCodeForUpdate call")
}

func (s *promoRepoStubForGiftCredit) Update(ctx context.Context, code *PromoCode) error {
	clone := *code
	s.updated = &clone
	return nil
}

func (s *promoRepoStubForGiftCredit) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (s *promoRepoStubForGiftCredit) List(ctx context.Context, params pagination.PaginationParams) ([]PromoCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *promoRepoStubForGiftCredit) ListWithFilters(ctx context.Context, params pagination.PaginationParams, status, search string) ([]PromoCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *promoRepoStubForGiftCredit) CreateUsage(ctx context.Context, usage *PromoCodeUsage) error {
	panic("unexpected CreateUsage call")
}

func (s *promoRepoStubForGiftCredit) GetUsageByPromoCodeAndUser(ctx context.Context, promoCodeID, userID int64) (*PromoCodeUsage, error) {
	panic("unexpected GetUsageByPromoCodeAndUser call")
}

func (s *promoRepoStubForGiftCredit) ListUsagesByPromoCode(ctx context.Context, promoCodeID int64, params pagination.PaginationParams) ([]PromoCodeUsage, *pagination.PaginationResult, error) {
	panic("unexpected ListUsagesByPromoCode call")
}

func (s *promoRepoStubForGiftCredit) IncrementUsedCount(ctx context.Context, id int64) error {
	panic("unexpected IncrementUsedCount call")
}

func TestPromoServiceCreateGiftRequiresExplicitValidityDays(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{}
	svc := NewPromoService(repo, nil, nil, nil, nil)

	_, err := svc.Create(context.Background(), &CreatePromoCodeInput{
		Code:        "GIFT0",
		BonusAmount: 1,
		CreditType:  creditTypeGift,
	})
	require.ErrorIs(t, err, ErrPromoGiftValidityRequired)
	require.Nil(t, repo.created)
}

func TestPromoServiceCreateGiftRejectsNegativeValidityDays(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{}
	svc := NewPromoService(repo, nil, nil, nil, nil)

	validityDays := -1
	_, err := svc.Create(context.Background(), &CreatePromoCodeInput{
		Code:             "GIFT_NEGATIVE",
		BonusAmount:      1,
		CreditType:       creditTypeGift,
		GiftValidityDays: &validityDays,
	})
	require.ErrorIs(t, err, ErrPromoGiftValidityRequired)
	require.Nil(t, repo.created)
}

func TestPromoServiceCreateRequiresExplicitCreditType(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{}
	svc := NewPromoService(repo, nil, nil, nil, nil)

	_, err := svc.Create(context.Background(), &CreatePromoCodeInput{
		Code:        "NO_TYPE",
		BonusAmount: 1,
	})
	require.ErrorIs(t, err, ErrPromoCreditTypeRequired)
	require.Nil(t, repo.created)
}

func TestPromoServiceCreateBalanceStoresExplicitCreditType(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{}
	svc := NewPromoService(repo, nil, nil, nil, nil)

	code, err := svc.Create(context.Background(), &CreatePromoCodeInput{
		Code:        "BALANCE",
		BonusAmount: 1,
		CreditType:  creditTypeBalance,
	})
	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Equal(t, creditTypeBalance, code.CreditType)
	require.Zero(t, code.GiftValidityDays)
	require.Contains(t, repo.created.Notes, `"credit_type":"balance"`)
}

func TestPromoServiceCreateGiftStoresExplicitValidityDays(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{}
	svc := NewPromoService(repo, nil, nil, nil, nil)

	validityDays := 7
	code, err := svc.Create(context.Background(), &CreatePromoCodeInput{
		Code:             "GIFT7",
		BonusAmount:      1,
		CreditType:       creditTypeGift,
		GiftValidityDays: &validityDays,
	})
	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Equal(t, creditTypeGift, code.CreditType)
	require.Equal(t, 7, code.GiftValidityDays)
	require.Contains(t, repo.created.Notes, `"gift_validity_days":7`)
}

func TestPromoServiceCreateGiftStoresPermanentValidityDays(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{}
	svc := NewPromoService(repo, nil, nil, nil, nil)

	validityDays := 0
	code, err := svc.Create(context.Background(), &CreatePromoCodeInput{
		Code:             "GIFT0",
		BonusAmount:      1,
		CreditType:       creditTypeGift,
		GiftValidityDays: &validityDays,
	})
	require.NoError(t, err)
	require.NotNil(t, repo.created)
	require.Equal(t, creditTypeGift, code.CreditType)
	require.Zero(t, code.GiftValidityDays)
	require.Contains(t, repo.created.Notes, `"gift_validity_days":0`)
}

func TestPromoServiceValidateGiftMissingValidityDays(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{
		byCode: &PromoCode{
			ID:          1,
			Code:        "GIFT_MISSING_VALIDITY",
			BonusAmount: 1,
			Status:      PromoCodeStatusActive,
			CreditType:  creditTypeGift,
		},
	}
	svc := NewPromoService(repo, nil, nil, nil, nil)

	_, err := svc.ValidatePromoCode(context.Background(), "GIFT_MISSING_VALIDITY")
	require.ErrorIs(t, err, ErrPromoGiftValidityRequired)
}

func TestPromoServiceValidateGiftAcceptsPermanentValidityDays(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{
		byCode: &PromoCode{
			ID:          1,
			Code:        "GIFT_PERMANENT",
			BonusAmount: 1,
			Status:      PromoCodeStatusActive,
			Notes:       `<!-- sub2api_custom_promo_meta:{"credit_type":"gift","gift_validity_days":0} -->`,
		},
	}
	svc := NewPromoService(repo, nil, nil, nil, nil)

	code, err := svc.ValidatePromoCode(context.Background(), "GIFT_PERMANENT")
	require.NoError(t, err)
	require.Equal(t, creditTypeGift, code.CreditType)
	require.Zero(t, code.GiftValidityDays)
}

func TestPromoServiceUpdateRejectsInvalidCreditType(t *testing.T) {
	repo := &promoRepoStubForGiftCredit{
		byID: &PromoCode{
			ID:          1,
			Code:        "BALANCE",
			BonusAmount: 1,
			Status:      PromoCodeStatusActive,
			CreditType:  creditTypeBalance,
		},
	}
	svc := NewPromoService(repo, nil, nil, nil, nil)
	invalid := "invalid"

	_, err := svc.Update(context.Background(), 1, &UpdatePromoCodeInput{CreditType: &invalid})
	require.ErrorIs(t, err, ErrPromoCreditTypeRequired)
	require.Nil(t, repo.updated)
}
