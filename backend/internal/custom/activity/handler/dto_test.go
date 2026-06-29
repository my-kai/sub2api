package handler

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
	"github.com/stretchr/testify/require"
)

func TestAdminActivityUpsertRequestRequiresExplicitGiftValidityDays(t *testing.T) {
	req := adminActivityUpsertRequest{
		Type:     types.ActivityTypeRedPacketRain,
		Title:    "红包雨",
		StartsAt: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 6, 18, 13, 0, 0, 0, time.UTC),
		RedPacketRain: adminRedPacketRainConfigInput{
			RoundCount:           1,
			RoundDurationSeconds: 60,
			TotalBudget:          "10.00000000",
			PerUserRoundCap:      "1.00000000",
			PerUserTotalCap:      "2.00000000",
			BaseUnitAmount:       "0.10000000",
			MaxSingleReward:      "1.00000000",
			ProbabilityStep:      "0.10000000",
		},
	}

	_, err := req.toServiceInput(0, 7)
	require.ErrorIs(t, err, types.ErrInvalidInput)
}

func TestAdminActivityUpsertRequestAllowsPermanentGiftValidityDays(t *testing.T) {
	validityDays := 0
	req := adminActivityUpsertRequest{
		Type:     types.ActivityTypeRedPacketRain,
		Title:    "红包雨",
		StartsAt: time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 6, 18, 13, 0, 0, 0, time.UTC),
		RedPacketRain: adminRedPacketRainConfigInput{
			RoundCount:           1,
			RoundDurationSeconds: 60,
			TotalBudget:          "10.00000000",
			PerUserRoundCap:      "1.00000000",
			PerUserTotalCap:      "2.00000000",
			BaseUnitAmount:       "0.10000000",
			MaxSingleReward:      "1.00000000",
			ProbabilityStep:      "0.10000000",
			GiftValidityDays:     &validityDays,
		},
	}

	input, err := req.toServiceInput(0, 7)
	require.NoError(t, err)
	require.Zero(t, input.RedPacketRain.GiftValidityDays)
}
