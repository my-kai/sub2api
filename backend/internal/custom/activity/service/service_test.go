package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestEffectiveActivityStatus(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	activity := types.Activity{
		Status:   types.ActivityStatusScheduled,
		StartsAt: now.Add(-time.Minute),
		EndsAt:   now.Add(time.Minute),
	}

	require.Equal(t, types.ActivityStatusActive, EffectiveActivityStatus(activity, now))
	require.Equal(t, types.ActivityStatusScheduled, EffectiveActivityStatus(activity, now.Add(-2*time.Minute)))
	require.Equal(t, types.ActivityStatusEnded, EffectiveActivityStatus(activity, now.Add(2*time.Minute)))

	activity.Status = types.ActivityStatusOffline
	require.Equal(t, types.ActivityStatusOffline, EffectiveActivityStatus(activity, now))
}

func TestCalculateRewardZeroHitNeverCredits(t *testing.T) {
	svc := NewService(nil).WithRandomSource(func() (decimal.Decimal, error) {
		return decimal.Zero, nil
	})
	reward, _, err := svc.calculateReward(t.Context(), testConfig(), 0, types.ClaimSummary{
		ActivityIssuedAmount: "0.00000000",
		UserRoundAmount:      "0.00000000",
		UserActivityAmount:   "0.00000000",
	})

	require.NoError(t, err)
	require.Equal(t, "0.00000000", amountString(reward))
}

func TestCalculateRewardHigherHitCountRaisesHighRewardChance(t *testing.T) {
	roll := decimal.RequireFromString("0.50")
	svc := NewService(nil).WithRandomSource(func() (decimal.Decimal, error) {
		return roll, nil
	})
	summary := types.ClaimSummary{
		ActivityIssuedAmount: "0.00000000",
		UserRoundAmount:      "0.00000000",
		UserActivityAmount:   "0.00000000",
	}

	lowHits, _, err := svc.calculateReward(t.Context(), testConfig(), 1, summary)
	require.NoError(t, err)
	highHits, _, err := svc.calculateReward(t.Context(), testConfig(), 8, summary)
	require.NoError(t, err)

	require.True(t, highHits.GreaterThan(lowHits), "more hits should have a higher chance to receive the high multiplier")
}

func TestCalculateRewardClampsToCapsAndBudget(t *testing.T) {
	svc := NewService(nil).WithRandomSource(func() (decimal.Decimal, error) {
		return decimal.Zero, nil
	})
	reward, _, err := svc.calculateReward(t.Context(), testConfig(), 10, types.ClaimSummary{
		ActivityIssuedAmount: "19.75000000",
		UserRoundAmount:      "4.90000000",
		UserActivityAmount:   "9.00000000",
	})

	require.NoError(t, err)
	require.Equal(t, "0.10000000", amountString(reward), "single settlement must not exceed the tightest remaining cap")
}

func TestBuildProgressReportsReachedCaps(t *testing.T) {
	userReward, budget, err := buildProgress(testConfig(), types.ClaimSummary{
		ActivityIssuedAmount: "20.00000000",
		UserRoundAmount:      "5.00000000",
		UserActivityAmount:   "10.00000000",
	})

	require.NoError(t, err)
	require.True(t, userReward.RoundCapReached)
	require.True(t, userReward.ActivityCapReached)
	require.True(t, budget.Exhausted)
	require.Equal(t, "0.00000000", budget.Remaining)
}

func testConfig() types.RedPacketRainConfig {
	return types.RedPacketRainConfig{
		RoundCount:           2,
		RoundDurationSeconds: 60,
		RoundIntervalSeconds: 900,
		TotalBudget:          "20.00000000",
		PerUserRoundCap:      "5.00000000",
		PerUserTotalCap:      "10.00000000",
		BaseUnitAmount:       "1.00000000",
		MaxSingleReward:      "4.00000000",
		ProbabilityStep:      "0.10000000",
		GiftValidityDays:     30,
	}
}
