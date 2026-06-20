package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
)

// EffectiveActivityStatus folds persisted status and time window into user-visible lifecycle.
func EffectiveActivityStatus(activity types.Activity, now time.Time) types.ActivityStatus {
	switch activity.Status {
	case types.ActivityStatusOffline:
		return types.ActivityStatusOffline
	case types.ActivityStatusEnded:
		return types.ActivityStatusEnded
	case types.ActivityStatusDraft:
		return types.ActivityStatusDraft
	}
	if now.Before(activity.StartsAt) {
		return types.ActivityStatusScheduled
	}
	if !now.Before(activity.EndsAt) {
		return types.ActivityStatusEnded
	}
	return types.ActivityStatusActive
}

// CurrentOrNextRound returns the active round when available, otherwise the next waiting round.
func CurrentOrNextRound(rounds []types.RedPacketRainRound, activityStatus types.ActivityStatus, now time.Time) *types.RedPacketRainRound {
	if activityStatus == types.ActivityStatusOffline {
		return nil
	}
	if activityStatus == types.ActivityStatusEnded {
		return nil
	}
	var next *types.RedPacketRainRound
	for i := range rounds {
		round := rounds[i]
		if now.Before(round.StartsAt) {
			if next == nil || round.StartsAt.Before(next.StartsAt) {
				candidate := round
				next = &candidate
			}
			continue
		}
		if now.Before(round.EndsAt) {
			active := round
			return &active
		}
	}
	return next
}

func roundStateFor(round types.RedPacketRainRound, activityStatus types.ActivityStatus, now time.Time) types.RedPacketRainRoundState {
	status := types.RoundStatusWaiting
	switch {
	case activityStatus == types.ActivityStatusOffline:
		status = types.RoundStatusOffline
	case activityStatus == types.ActivityStatusEnded:
		status = types.RoundStatusFinished
	case now.Before(round.StartsAt):
		status = types.RoundStatusWaiting
	case now.Before(round.EndsAt):
		status = types.RoundStatusActive
	default:
		status = types.RoundStatusEnded
	}
	return types.RedPacketRainRoundState{
		ID:                round.ID,
		RoundNo:           round.RoundNo,
		Status:            status,
		StartsAt:          round.StartsAt,
		EndsAt:            round.EndsAt,
		ServerNow:         now,
		SecondsUntilStart: secondsUntil(now, round.StartsAt),
		SecondsUntilEnd:   secondsUntil(now, round.EndsAt),
	}
}

func secondsUntil(now time.Time, target time.Time) int64 {
	if !target.After(now) {
		return 0
	}
	return int64(target.Sub(now).Seconds())
}
