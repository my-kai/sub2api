package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/security"
	"github.com/Wei-Shaw/sub2api/internal/custom/activity/store"
	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
	"github.com/shopspring/decimal"
)

const defaultBalanceCacheTimeout = 2 * time.Second

// BalanceCacheInvalidator is the narrow main-repo cache hook needed after rewards are credited.
type BalanceCacheInvalidator interface {
	InvalidateUserBalance(ctx context.Context, userID int64) error
}

// AuthCacheInvalidator is the narrow main-repo auth cache hook needed after balance changes.
type AuthCacheInvalidator interface {
	InvalidateAuthCacheByUserID(ctx context.Context, userID int64)
}

// RandomSource returns a value in [0, 1) for reward factor calculation.
type RandomSource func() (decimal.Decimal, error)

// Service owns custom activity business rules.
type Service struct {
	store                   *store.Store
	now                     func() time.Time
	random                  RandomSource
	balanceCacheInvalidator BalanceCacheInvalidator
	authCacheInvalidator    AuthCacheInvalidator
	rateLimiter             *security.RateLimiter
	logger                  *slog.Logger
}

// NewService builds the activity service around the custom store.
func NewService(activityStore *store.Store) *Service {
	return &Service{
		store:       activityStore,
		now:         func() time.Time { return time.Now().UTC() },
		random:      cryptoRandomDecimal,
		rateLimiter: security.NewRateLimiter(),
		logger:      slog.Default(),
	}
}

// WithClock injects a deterministic clock for tests and state snapshots.
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// WithRandomSource injects a deterministic random source for reward tests.
func (s *Service) WithRandomSource(random RandomSource) *Service {
	if random != nil {
		s.random = random
	}
	return s
}

// WithBalanceCacheInvalidator wires billing balance cache invalidation after successful credits.
func (s *Service) WithBalanceCacheInvalidator(invalidator BalanceCacheInvalidator) *Service {
	s.balanceCacheInvalidator = invalidator
	return s
}

// WithAuthCacheInvalidator wires auth cache invalidation after successful credits.
func (s *Service) WithAuthCacheInvalidator(invalidator AuthCacheInvalidator) *Service {
	s.authCacheInvalidator = invalidator
	return s
}

// ActivityStore exposes the persistence dependency for thin handlers.
func (s *Service) ActivityStore() *store.Store {
	if s == nil {
		return nil
	}
	return s.store
}

// GetRedPacketRainState returns the current user-facing activity and round status.
func (s *Service) GetRedPacketRainState(ctx context.Context, activityID int64, userID int64) (types.RedPacketRainState, error) {
	if s == nil || s.store == nil {
		return types.RedPacketRainState{}, fmt.Errorf("custom activity service is not configured")
	}
	activity, cfg, rounds, err := s.loadActivityConfigRounds(ctx, activityID)
	if err != nil {
		return types.RedPacketRainState{}, err
	}
	now := s.now().UTC()
	status := EffectiveActivityStatus(activity, now)
	round := CurrentOrNextRound(rounds, status, now)

	summary := types.ClaimSummary{}
	roundID := int64(0)
	if round != nil {
		roundID = round.ID
	}
	if roundID > 0 && userID > 0 {
		summary, err = s.store.ClaimSummary(ctx, activityID, roundID, userID)
		if err != nil {
			return types.RedPacketRainState{}, err
		}
	}
	userReward, budget, err := buildProgress(cfg, summary)
	if err != nil {
		return types.RedPacketRainState{}, err
	}

	stateStatus := types.RoundStatusWaiting
	var roundState *types.RedPacketRainRoundState
	if round != nil {
		state := roundStateFor(*round, status, now)
		roundState = &state
		stateStatus = state.Status
	} else if status == types.ActivityStatusOffline {
		stateStatus = types.RoundStatusOffline
	} else if status == types.ActivityStatusEnded {
		stateStatus = types.RoundStatusFinished
	}
	return types.RedPacketRainState{
		ActivityID:  activityID,
		Status:      stateStatus,
		Round:       roundState,
		UserReward:  userReward,
		Budget:      budget,
		ServerNow:   now,
		FinishedAll: status == types.ActivityStatusEnded || (roundState != nil && roundState.Status == types.RoundStatusFinished),
	}, nil
}

// ClaimRedPacketRain validates and settles one red packet rain claim.
func (s *Service) ClaimRedPacketRain(ctx context.Context, input ClaimInput) (types.ClaimResult, error) {
	if s == nil || s.store == nil {
		return types.ClaimResult{}, fmt.Errorf("custom activity service is not configured")
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.ActivityID <= 0 || input.RoundID <= 0 || input.UserID <= 0 || input.HitCount < 0 || input.IdempotencyKey == "" {
		return types.ClaimResult{}, types.ErrInvalidInput
	}

	activity, cfg, rounds, err := s.loadActivityConfigRounds(ctx, input.ActivityID)
	if err != nil {
		return types.ClaimResult{}, err
	}
	now := s.now().UTC()
	if err := validateClaimWindow(activity, rounds, input.RoundID, now); err != nil {
		return types.ClaimResult{}, err
	}
	if input.HitCount == 0 {
		summary, err := s.store.ClaimSummary(ctx, input.ActivityID, input.RoundID, input.UserID)
		if err != nil {
			return types.ClaimResult{}, err
		}
		userReward, budget, err := buildProgress(cfg, summary)
		if err != nil {
			return types.ClaimResult{}, err
		}
		return types.ClaimResult{
			ActivityID:   input.ActivityID,
			RoundID:      input.RoundID,
			HitCount:     0,
			RewardAmount: amountString(zeroAmount),
			Credited:     false,
			Duplicate:    false,
			Message:      claimMessage(amountString(zeroAmount), userReward, budget),
			UserReward:   userReward,
			Budget:       budget,
		}, nil
	}

	txResult, err := s.store.SettleClaim(ctx, store.ClaimTransactionInput{
		ActivityID:       input.ActivityID,
		RoundID:          input.RoundID,
		UserID:           input.UserID,
		HitCount:         input.HitCount,
		IdempotencyKey:   input.IdempotencyKey,
		ActivityTitle:    activity.Title,
		CreatedAt:        now,
		GiftValidityDays: cfg.GiftValidityDays,
	}, func(summary types.ClaimSummary) (store.ClaimRewardDecision, error) {
		reward, capMessage, calcErr := s.calculateReward(ctx, cfg, input.HitCount, summary)
		if calcErr != nil {
			return store.ClaimRewardDecision{}, calcErr
		}
		_ = capMessage // message is derived again from final progress for duplicate-safe output.
		return store.ClaimRewardDecision{
			RewardAmount:      amountString(reward),
			CreditUserBalance: reward.GreaterThan(zeroAmount),
		}, nil
	})
	if err != nil {
		return types.ClaimResult{}, err
	}

	userReward, budget, err := buildProgress(cfg, txResult.Summary)
	if err != nil {
		return types.ClaimResult{}, err
	}
	result := types.ClaimResult{
		ClaimID:      txResult.Claim.ID,
		ActivityID:   txResult.Claim.ActivityID,
		RoundID:      txResult.Claim.RoundID,
		HitCount:     txResult.Claim.HitCount,
		RewardAmount: txResult.Claim.RewardAmount,
		Credited:     amountPositive(txResult.Claim.RewardAmount),
		Duplicate:    txResult.Duplicate,
		Message:      claimMessage(txResult.Claim.RewardAmount, userReward, budget),
		UserReward:   userReward,
		Budget:       budget,
	}
	if result.Credited && !txResult.Duplicate {
		s.invalidateBalanceCaches(ctx, input.UserID)
	}
	return result, nil
}

// ClaimInput is the service request for one claim.
type ClaimInput struct {
	ActivityID     int64
	RoundID        int64
	UserID         int64
	HitCount       int
	IdempotencyKey string
}

func (s *Service) loadActivityConfigRounds(ctx context.Context, activityID int64) (types.Activity, types.RedPacketRainConfig, []types.RedPacketRainRound, error) {
	activity, err := s.store.GetActivity(ctx, activityID)
	if err != nil {
		return types.Activity{}, types.RedPacketRainConfig{}, nil, err
	}
	if activity.Type != types.ActivityTypeRedPacketRain {
		return types.Activity{}, types.RedPacketRainConfig{}, nil, types.ErrInvalidInput
	}
	cfg, err := s.store.GetRedPacketRainConfig(ctx, activityID)
	if err != nil {
		return types.Activity{}, types.RedPacketRainConfig{}, nil, err
	}
	rounds, err := s.store.ListRounds(ctx, activityID)
	if err != nil {
		return types.Activity{}, types.RedPacketRainConfig{}, nil, err
	}
	return activity, cfg, rounds, nil
}

func validateClaimWindow(activity types.Activity, rounds []types.RedPacketRainRound, roundID int64, now time.Time) error {
	switch EffectiveActivityStatus(activity, now) {
	case types.ActivityStatusOffline:
		return types.ErrActivityOffline
	case types.ActivityStatusDraft, types.ActivityStatusScheduled:
		return types.ErrActivityNotStarted
	case types.ActivityStatusEnded:
		return types.ErrActivityEnded
	}
	for _, round := range rounds {
		if round.ID != roundID {
			continue
		}
		if round.Status == types.RoundStatusOffline {
			return types.ErrActivityOffline
		}
		if now.Before(round.StartsAt) {
			return types.ErrRoundNotStarted
		}
		if !now.Before(round.EndsAt) {
			return types.ErrRoundEnded
		}
		return nil
	}
	return types.ErrRoundEnded
}

func (s *Service) calculateReward(ctx context.Context, cfg types.RedPacketRainConfig, hitCount int, summary types.ClaimSummary) (decimal.Decimal, string, error) {
	if hitCount == 0 {
		return decimal.Zero, "未获得奖励", nil
	}
	progress, err := parseProgress(cfg, summary)
	if err != nil {
		return decimal.Zero, "", err
	}
	if progress.roundRemaining.IsZero() {
		return decimal.Zero, "本轮领取已达上限", nil
	}
	if progress.activityRemaining.IsZero() {
		return decimal.Zero, "活动领取已达上限", nil
	}
	if progress.budgetRemaining.IsZero() {
		return decimal.Zero, "活动奖励已发完", nil
	}

	factor, err := s.randomFactor(ctx, cfg, hitCount)
	if err != nil {
		return decimal.Zero, "", err
	}
	base, _ := parseAmount(cfg.BaseUnitAmount)
	maxSingle, _ := parseAmount(cfg.MaxSingleReward)
	raw := base.Mul(decimal.NewFromInt(int64(hitCount))).Mul(factor)
	reward := minAmount(raw, maxSingle, progress.roundRemaining, progress.activityRemaining, progress.budgetRemaining)
	if reward.IsNegative() {
		reward = decimal.Zero
	}
	return reward.Round(8), "", nil
}

func (s *Service) randomFactor(ctx context.Context, cfg types.RedPacketRainConfig, hitCount int) (decimal.Decimal, error) {
	step, err := parseAmount(cfg.ProbabilityStep)
	if err != nil {
		return decimal.Zero, err
	}
	effectiveProbability := decimal.NewFromInt(int64(hitCount)).Mul(step)
	if effectiveProbability.GreaterThan(oneAmount) {
		effectiveProbability = oneAmount
	}
	roll, err := s.random()
	if err != nil {
		return decimal.Zero, fmt.Errorf("generate red packet rain random factor: %w", err)
	}
	if roll.LessThan(effectiveProbability) {
		// Higher hit_count increases the chance of drawing a high multiplier,
		// while the exact multiplier still stays server-side and testable.
		return decimal.NewFromInt(2).Add(effectiveProbability), nil
	}
	return oneAmount, nil
}

type rewardProgress struct {
	roundRemaining    decimal.Decimal
	activityRemaining decimal.Decimal
	budgetRemaining   decimal.Decimal
}

func parseProgress(cfg types.RedPacketRainConfig, summary types.ClaimSummary) (rewardProgress, error) {
	totalBudget, err := parseAmount(cfg.TotalBudget)
	if err != nil {
		return rewardProgress{}, err
	}
	roundCap, err := parseAmount(cfg.PerUserRoundCap)
	if err != nil {
		return rewardProgress{}, err
	}
	totalCap, err := parseAmount(cfg.PerUserTotalCap)
	if err != nil {
		return rewardProgress{}, err
	}
	issued, err := parseSummaryAmount(summary.ActivityIssuedAmount)
	if err != nil {
		return rewardProgress{}, err
	}
	userRound, err := parseSummaryAmount(summary.UserRoundAmount)
	if err != nil {
		return rewardProgress{}, err
	}
	userActivity, err := parseSummaryAmount(summary.UserActivityAmount)
	if err != nil {
		return rewardProgress{}, err
	}
	return rewardProgress{
		roundRemaining:    positiveRemaining(roundCap, userRound),
		activityRemaining: positiveRemaining(totalCap, userActivity),
		budgetRemaining:   positiveRemaining(totalBudget, issued),
	}, nil
}

func buildProgress(cfg types.RedPacketRainConfig, summary types.ClaimSummary) (types.UserRewardState, types.BudgetState, error) {
	progress, err := parseProgress(cfg, summary)
	if err != nil {
		return types.UserRewardState{}, types.BudgetState{}, err
	}
	return types.UserRewardState{
			RoundTotal:         amountString(mustAmount(summary.UserRoundAmount)),
			ActivityTotal:      amountString(mustAmount(summary.UserActivityAmount)),
			RoundRemaining:     amountString(progress.roundRemaining),
			ActivityRemaining:  amountString(progress.activityRemaining),
			RoundCapReached:    progress.roundRemaining.IsZero(),
			ActivityCapReached: progress.activityRemaining.IsZero(),
		}, types.BudgetState{
			Remaining: amountString(progress.budgetRemaining),
			Exhausted: progress.budgetRemaining.IsZero(),
		}, nil
}

func mustAmount(value string) decimal.Decimal {
	amount, err := parseSummaryAmount(value)
	if err != nil {
		return decimal.Zero
	}
	return amount
}

func parseSummaryAmount(value string) (decimal.Decimal, error) {
	if strings.TrimSpace(value) == "" {
		return decimal.Zero, nil
	}
	return parseAmount(value)
}

func amountPositive(value string) bool {
	amount, err := parseSummaryAmount(value)
	return err == nil && amount.GreaterThan(zeroAmount)
}

func claimMessage(rewardAmount string, userReward types.UserRewardState, budget types.BudgetState) string {
	if rewardAmount != amountString(zeroAmount) {
		return "奖励已到账"
	}
	if budget.Exhausted {
		return "活动奖励已发完"
	}
	if userReward.ActivityCapReached {
		return "活动领取已达上限"
	}
	if userReward.RoundCapReached {
		return "本轮领取已达上限"
	}
	return "未获得奖励"
}

func (s *Service) invalidateBalanceCaches(ctx context.Context, userID int64) {
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	if s.balanceCacheInvalidator == nil {
		return
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultBalanceCacheTimeout)
	defer cancel()
	if err := s.balanceCacheInvalidator.InvalidateUserBalance(cacheCtx, userID); err != nil && s.logger != nil {
		s.logger.Warn("custom activity balance cache invalidation failed", "user_id", userID, "error", err)
	}
}

func cryptoRandomDecimal() (decimal.Decimal, error) {
	max := big.NewInt(1_000_000)
	value, err := rand.Int(rand.Reader, max)
	if err != nil {
		return decimal.Zero, err
	}
	return decimal.NewFromBigInt(value, 0).Div(decimal.NewFromInt(max.Int64())), nil
}

// ErrorMessage maps service errors to short business messages for handlers.
func ErrorMessage(err error) string {
	switch {
	case errors.Is(err, types.ErrActivityNotStarted):
		return "活动未开始"
	case errors.Is(err, types.ErrActivityEnded):
		return "活动已结束"
	case errors.Is(err, types.ErrActivityOffline):
		return "活动已下架"
	case errors.Is(err, types.ErrRoundNotStarted):
		return "本轮未开始"
	case errors.Is(err, types.ErrRoundEnded):
		return "本轮已结束"
	case errors.Is(err, types.ErrUserRoundCapReached):
		return "本轮领取已达上限"
	case errors.Is(err, types.ErrUserTotalCapReached):
		return "活动领取已达上限"
	case errors.Is(err, types.ErrBudgetExhausted):
		return "活动奖励已发完"
	case errors.Is(err, types.ErrRedPacketRainSecurityRejected):
		return "领取失败"
	default:
		return "操作失败"
	}
}
