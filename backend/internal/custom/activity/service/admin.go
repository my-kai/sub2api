package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/activity/store"
	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
)

// UpsertActivityInput is the admin request for creating or editing a red packet rain activity.
type UpsertActivityInput struct {
	ID            int64
	Type          types.ActivityType
	Title         string
	Description   string
	CoverURL      string
	StartsAt      time.Time
	EndsAt        time.Time
	CreatedBy     int64
	RedPacketRain types.RedPacketRainConfig
}

// ActivityDetail combines common activity fields with red packet rain config and rounds.
type ActivityDetail struct {
	Activity        types.Activity
	Config          types.RedPacketRainConfig
	Rounds          []types.RedPacketRainRound
	RoundSummaries  map[int64]store.RoundClaimSummary
	Summary         types.ActivityAdminSummary
	ClaimSummary    types.ClaimSummary
	EffectiveStatus types.ActivityStatus
}

// CreateRedPacketRainActivity creates one activity and its precomputed round windows.
func (s *Service) CreateRedPacketRainActivity(ctx context.Context, input UpsertActivityInput) (ActivityDetail, error) {
	if err := validateUpsertInput(input); err != nil {
		return ActivityDetail{}, err
	}
	now := s.now().UTC()
	status := types.ActivityStatusScheduled
	if now.After(input.EndsAt) || now.Equal(input.EndsAt) {
		status = types.ActivityStatusEnded
	}
	activity, err := s.store.CreateActivity(ctx, types.Activity{
		Type:        types.ActivityTypeRedPacketRain,
		Title:       input.Title,
		Description: input.Description,
		CoverURL:    input.CoverURL,
		Status:      status,
		StartsAt:    input.StartsAt,
		EndsAt:      input.EndsAt,
		CreatedBy:   input.CreatedBy,
		CreatedAt:   now,
	})
	if err != nil {
		return ActivityDetail{}, err
	}
	cfg := input.RedPacketRain
	cfg.ActivityID = activity.ID
	cfg.CreatedAt = now
	cfg.UpdatedAt = now
	if _, err := s.store.UpsertRedPacketRainConfig(ctx, cfg); err != nil {
		return ActivityDetail{}, err
	}
	if err := s.store.ReplaceRounds(ctx, activity.ID, GenerateRounds(activity, cfg, now)); err != nil {
		return ActivityDetail{}, err
	}
	return s.GetAdminActivityDetail(ctx, activity.ID)
}

// UpdateRedPacketRainActivity updates an activity that has not started yet.
func (s *Service) UpdateRedPacketRainActivity(ctx context.Context, input UpsertActivityInput) (ActivityDetail, error) {
	if input.ID <= 0 {
		return ActivityDetail{}, types.ErrInvalidInput
	}
	if err := validateUpsertInput(input); err != nil {
		return ActivityDetail{}, err
	}
	existing, err := s.store.GetActivity(ctx, input.ID)
	if err != nil {
		return ActivityDetail{}, err
	}
	if EffectiveActivityStatus(existing, s.now().UTC()) == types.ActivityStatusActive || EffectiveActivityStatus(existing, s.now().UTC()) == types.ActivityStatusEnded {
		return ActivityDetail{}, types.ErrActivityEnded
	}
	existing.Title = input.Title
	existing.Description = input.Description
	existing.CoverURL = input.CoverURL
	existing.StartsAt = input.StartsAt
	existing.EndsAt = input.EndsAt
	existing.Status = types.ActivityStatusScheduled
	existing.UpdatedAt = s.now().UTC()
	if _, err := s.store.UpdateActivity(ctx, existing); err != nil {
		return ActivityDetail{}, err
	}
	cfg := input.RedPacketRain
	cfg.ActivityID = input.ID
	cfg.UpdatedAt = s.now().UTC()
	if _, err := s.store.UpsertRedPacketRainConfig(ctx, cfg); err != nil {
		return ActivityDetail{}, err
	}
	if err := s.store.ReplaceRounds(ctx, input.ID, GenerateRounds(existing, cfg, s.now().UTC())); err != nil {
		return ActivityDetail{}, err
	}
	return s.GetAdminActivityDetail(ctx, input.ID)
}

// ListAdminSummaries returns paginated admin summaries.
func (s *Service) ListAdminSummaries(ctx context.Context, page types.PageRequest) ([]types.ActivityAdminSummary, int64, error) {
	return s.store.ListAdminSummaries(ctx, page)
}

// ListVisibleActivities returns activity cards available to users.
func (s *Service) ListVisibleActivities(ctx context.Context, page types.PageRequest) ([]types.Activity, int64, error) {
	return s.store.ListActivities(ctx, []types.ActivityStatus{types.ActivityStatusScheduled, types.ActivityStatusActive, types.ActivityStatusEnded}, page)
}

// GetAdminActivityDetail loads all details required by the admin page.
func (s *Service) GetAdminActivityDetail(ctx context.Context, activityID int64) (ActivityDetail, error) {
	activity, cfg, rounds, err := s.loadActivityConfigRounds(ctx, activityID)
	if err != nil {
		return ActivityDetail{}, err
	}
	summary, err := s.store.ClaimSummary(ctx, activityID, 0, 0)
	if err != nil {
		return ActivityDetail{}, err
	}
	roundSummaries, err := s.store.RoundClaimSummaries(ctx, activityID)
	if err != nil {
		return ActivityDetail{}, err
	}
	return ActivityDetail{
		Activity:        activity,
		Config:          cfg,
		Rounds:          rounds,
		RoundSummaries:  roundSummaries,
		ClaimSummary:    summary,
		EffectiveStatus: EffectiveActivityStatus(activity, s.now().UTC()),
	}, nil
}

// EndActivity ends an activity early without touching historical claims.
func (s *Service) EndActivity(ctx context.Context, activityID int64) (types.Activity, error) {
	now := s.now().UTC()
	return s.store.SetActivityStatus(ctx, activityID, types.ActivityStatusEnded, &now, now)
}

// OfflineActivity hides an activity from user-facing pages.
func (s *Service) OfflineActivity(ctx context.Context, activityID int64) (types.Activity, error) {
	now := s.now().UTC()
	return s.store.SetActivityStatus(ctx, activityID, types.ActivityStatusOffline, nil, now)
}

// GenerateRounds builds deterministic round windows from the activity start time.
func GenerateRounds(activity types.Activity, cfg types.RedPacketRainConfig, now time.Time) []types.RedPacketRainRound {
	rounds := make([]types.RedPacketRainRound, 0, cfg.RoundCount)
	start := activity.StartsAt.UTC()
	for i := 1; i <= cfg.RoundCount; i++ {
		end := start.Add(time.Duration(cfg.RoundDurationSeconds) * time.Second)
		rounds = append(rounds, types.RedPacketRainRound{
			ActivityID: activity.ID,
			RoundNo:    i,
			StartsAt:   start,
			EndsAt:     end,
			Status:     types.RoundStatusWaiting,
			CreatedAt:  now.UTC(),
		})
		start = end.Add(time.Duration(cfg.RoundIntervalSeconds) * time.Second)
	}
	return rounds
}

func validateUpsertInput(input UpsertActivityInput) error {
	if input.Type != "" && input.Type != types.ActivityTypeRedPacketRain {
		return types.ErrInvalidInput
	}
	if input.Title == "" || !input.EndsAt.After(input.StartsAt) {
		return types.ErrInvalidInput
	}
	cfg := input.RedPacketRain
	if cfg.RoundCount <= 0 || cfg.RoundDurationSeconds <= 0 || cfg.RoundIntervalSeconds < 0 {
		return types.ErrInvalidInput
	}
	if cfg.GiftValidityDays <= 0 {
		return types.ErrInvalidInput
	}
	totalBudget, err := parseAmount(cfg.TotalBudget)
	if err != nil || !totalBudget.GreaterThan(zeroAmount) {
		return types.ErrInvalidInput
	}
	roundCap, err := parseAmount(cfg.PerUserRoundCap)
	if err != nil || !roundCap.GreaterThan(zeroAmount) {
		return types.ErrInvalidInput
	}
	totalCap, err := parseAmount(cfg.PerUserTotalCap)
	if err != nil || !totalCap.GreaterThan(zeroAmount) || totalCap.GreaterThan(totalBudget) {
		return types.ErrInvalidInput
	}
	base, err := parseAmount(cfg.BaseUnitAmount)
	if err != nil || !base.GreaterThan(zeroAmount) {
		return types.ErrInvalidInput
	}
	maxSingle, err := parseAmount(cfg.MaxSingleReward)
	if err != nil || !maxSingle.GreaterThan(zeroAmount) || maxSingle.GreaterThan(roundCap) {
		return types.ErrInvalidInput
	}
	step, err := parseAmount(cfg.ProbabilityStep)
	if err != nil || !step.GreaterThan(zeroAmount) || step.GreaterThan(oneAmount) {
		return types.ErrInvalidInput
	}
	return nil
}
