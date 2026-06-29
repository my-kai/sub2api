//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	gifttypes "github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/types"
	"github.com/stretchr/testify/require"
)

type balanceUserRepoStub struct {
	*userRepoStub
	updateErr error
	updated   []*User
}

func (s *balanceUserRepoStub) Update(ctx context.Context, user *User) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	if user == nil {
		return nil
	}
	clone := *user
	s.updated = append(s.updated, &clone)
	if s.userRepoStub != nil {
		s.userRepoStub.user = &clone
	}
	return nil
}

type balanceRedeemRepoStub struct {
	*redeemRepoStub
	created []*RedeemCode
}

func (s *balanceRedeemRepoStub) Create(ctx context.Context, code *RedeemCode) error {
	if code == nil {
		return nil
	}
	clone := *code
	s.created = append(s.created, &clone)
	return nil
}

type authCacheInvalidatorStub struct {
	userIDs  []int64
	groupIDs []int64
	keys     []string
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	s.keys = append(s.keys, key)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *authCacheInvalidatorStub) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	s.groupIDs = append(s.groupIDs, groupID)
}

type giftCreditGrantCreatorStub struct {
	inputs []gifttypes.CreateGrantInput
}

func (s *giftCreditGrantCreatorStub) CreateGrant(ctx context.Context, input gifttypes.CreateGrantInput) (gifttypes.Grant, error) {
	s.inputs = append(s.inputs, input)
	return gifttypes.Grant{
		ID:              1,
		UserID:          input.UserID,
		SourceType:      input.SourceType,
		SourceID:        input.SourceID,
		OriginalAmount:  input.Amount,
		RemainingAmount: input.Amount,
		ExpiresAt:       input.ExpiresAt,
		Status:          gifttypes.StatusActive,
		CreatedAt:       input.CreatedAt,
		UpdatedAt:       input.CreatedAt,
	}, nil
}

func TestAdminService_UpdateUserBalance_InvalidatesAuthCache(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
	}

	_, err := svc.UpdateUserBalance(context.Background(), 7, 5, "add", "")
	require.NoError(t, err)
	require.Equal(t, []int64{7}, invalidator.userIDs)
	require.Len(t, redeemRepo.created, 1)
}

func TestAdminService_UpdateUserBalance_NoChangeNoInvalidate(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{
		userRepo:             repo,
		redeemCodeRepo:       redeemRepo,
		authCacheInvalidator: invalidator,
	}

	_, err := svc.UpdateUserBalance(context.Background(), 7, 10, "set", "")
	require.NoError(t, err)
	require.Empty(t, invalidator.userIDs)
	require.Empty(t, redeemRepo.created)
}

func TestAdminService_UpdateUserBalanceGiftRequiresValidityDays(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	creator := &giftCreditGrantCreatorStub{}
	svc := &adminServiceImpl{
		userRepo:               repo,
		giftCreditGrantCreator: creator,
	}

	_, err := svc.UpdateUserBalanceWithOptions(context.Background(), 7, UpdateUserBalanceInput{
		Balance:    5,
		Operation:  "add",
		CreditType: "gift",
	})
	require.ErrorContains(t, err, "gift credit validity days is required and cannot be negative")
	require.Empty(t, creator.inputs)
}

func TestAdminService_UpdateUserBalanceGiftRejectsNegativeValidityDays(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	creator := &giftCreditGrantCreatorStub{}
	svc := &adminServiceImpl{
		userRepo:               repo,
		giftCreditGrantCreator: creator,
	}

	validityDays := -1
	_, err := svc.UpdateUserBalanceWithOptions(context.Background(), 7, UpdateUserBalanceInput{
		Balance:          5,
		Operation:        "add",
		CreditType:       "gift",
		GiftValidityDays: &validityDays,
	})
	require.ErrorIs(t, err, ErrAdminGiftValidityRequired)
	require.Empty(t, creator.inputs)
}

func TestAdminService_UpdateUserBalanceWithOptionsRequiresExplicitCreditType(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	svc := &adminServiceImpl{
		userRepo: repo,
	}

	_, err := svc.UpdateUserBalanceWithOptions(context.Background(), 7, UpdateUserBalanceInput{
		Balance:   5,
		Operation: "add",
	})
	require.ErrorIs(t, err, ErrAdminBalanceCreditTypeRequired)
	require.Empty(t, repo.updated)
}

func TestAdminService_UpdateUserBalanceGiftCreatesGrantWithExplicitValidity(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	creator := &giftCreditGrantCreatorStub{}
	invalidator := &authCacheInvalidatorStub{}
	validityDays := 3
	svc := &adminServiceImpl{
		userRepo:               repo,
		redeemCodeRepo:         redeemRepo,
		giftCreditGrantCreator: creator,
		authCacheInvalidator:   invalidator,
	}

	before := time.Now().UTC()
	user, err := svc.UpdateUserBalanceWithOptions(context.Background(), 7, UpdateUserBalanceInput{
		Balance:          5,
		Operation:        "add",
		CreditType:       "gift",
		GiftValidityDays: &validityDays,
		Notes:            "manual gift",
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), user.ID)
	require.Len(t, creator.inputs, 1)
	require.Equal(t, gifttypes.SourceAdminGrant, creator.inputs[0].SourceType)
	require.Equal(t, "5.00000000", creator.inputs[0].Amount)
	require.NotNil(t, creator.inputs[0].ExpiresAt)
	require.WithinDuration(t, before.AddDate(0, 0, 3), *creator.inputs[0].ExpiresAt, 2*time.Second)
	require.Equal(t, []int64{7}, invalidator.userIDs)
	require.Len(t, redeemRepo.created, 1)
}

func TestAdminService_UpdateUserBalanceGiftCreatesPermanentGrantWithZeroValidity(t *testing.T) {
	baseRepo := &userRepoStub{user: &User{ID: 7, Balance: 10}}
	repo := &balanceUserRepoStub{userRepoStub: baseRepo}
	redeemRepo := &balanceRedeemRepoStub{redeemRepoStub: &redeemRepoStub{}}
	creator := &giftCreditGrantCreatorStub{}
	svc := &adminServiceImpl{
		userRepo:               repo,
		redeemCodeRepo:         redeemRepo,
		giftCreditGrantCreator: creator,
	}

	validityDays := 0
	_, err := svc.UpdateUserBalanceWithOptions(context.Background(), 7, UpdateUserBalanceInput{
		Balance:          5,
		Operation:        "add",
		CreditType:       "gift",
		GiftValidityDays: &validityDays,
	})
	require.NoError(t, err)
	require.Len(t, creator.inputs, 1)
	require.Nil(t, creator.inputs[0].ExpiresAt)
}
