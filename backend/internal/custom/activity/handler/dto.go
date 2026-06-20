package handler

import (
	"strings"
	"time"

	activityservice "github.com/Wei-Shaw/sub2api/internal/custom/activity/service"
	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
)

type userActivitySummary struct {
	TotalBudget     string `json:"total_budget"`
	UserTotalReward string `json:"user_total_reward"`
}

type userActivityItem struct {
	ID          int64                `json:"id"`
	Type        types.ActivityType   `json:"type"`
	Title       string               `json:"title"`
	Description string               `json:"description"`
	CoverURL    string               `json:"cover_url"`
	Status      types.ActivityStatus `json:"status"`
	StartsAt    time.Time            `json:"starts_at"`
	EndsAt      time.Time            `json:"ends_at"`
	Summary     userActivitySummary  `json:"summary"`
}

type userActivityDetail struct {
	userActivityItem
	RedPacketRain *userRedPacketRainConfig `json:"red_packet_rain,omitempty"`
}

type userRedPacketRainConfig struct {
	RoundCount           int    `json:"round_count"`
	RoundDurationSeconds int    `json:"round_duration_seconds"`
	RoundIntervalSeconds int    `json:"round_interval_seconds"`
	PerUserRoundCap      string `json:"per_user_round_cap"`
	PerUserTotalCap      string `json:"per_user_total_cap"`
}

type claimRequest struct {
	RoundID        int64  `json:"round_id"`
	HitCount       int    `json:"hit_count"`
	IdempotencyKey string `json:"idempotency_key"`
}

type wsTicketRequest struct {
	RoundID             int64  `json:"round_id"`
	DeviceFingerprint   string `json:"device_fingerprint"`
	ClientNonce         string `json:"client_nonce"`
}

type wsMessage struct {
	Type string `json:"type"`
}

type wsChallengeMessage struct {
	Type        string    `json:"type"`
	SessionID   string    `json:"session_id"`
	ServerNonce string    `json:"server_nonce"`
	Challenge   string    `json:"challenge"`
	ExpiresAt   time.Time `json:"expires_at"`
	RoundID     int64     `json:"round_id"`
	RoundEndsAt time.Time `json:"round_ends_at"`
}

type wsClaimMessage struct {
	Type           string `json:"type"`
	SessionID      string `json:"session_id"`
	RoundID        int64  `json:"round_id"`
	IdempotencyKey string `json:"idempotency_key"`
	Nonce          string `json:"nonce"`
	Ciphertext     string `json:"ciphertext"`
	Signature      string `json:"signature"`
}

type wsClaimResultMessage struct {
	Type string            `json:"type"`
	Data types.ClaimResult `json:"data"`
}

type wsErrorMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type adminActivityItem struct {
	ID               int64                `json:"id"`
	Type             types.ActivityType   `json:"type"`
	Title            string               `json:"title"`
	Status           types.ActivityStatus `json:"status"`
	StartsAt         time.Time            `json:"starts_at"`
	EndsAt           time.Time            `json:"ends_at"`
	TotalBudget      string               `json:"total_budget"`
	IssuedAmount     string               `json:"issued_amount"`
	ParticipantCount int64                `json:"participant_count"`
}

type adminActivityDetail struct {
	adminActivityItem
	Description   string                     `json:"description"`
	CoverURL      string                     `json:"cover_url"`
	RedPacketRain *types.RedPacketRainConfig `json:"red_packet_rain,omitempty"`
	Rounds        []adminRoundItem           `json:"rounds"`
}

type adminRoundItem struct {
	ID               int64             `json:"id"`
	RoundNo          int               `json:"round_no"`
	Status           types.RoundStatus `json:"status"`
	StartsAt         time.Time         `json:"starts_at"`
	EndsAt           time.Time         `json:"ends_at"`
	IssuedAmount     string            `json:"issued_amount"`
	ParticipantCount int64             `json:"participant_count"`
	ClaimCount       int64             `json:"claim_count"`
}

type adminClaimItem struct {
	ID           int64     `json:"id"`
	RoundNo      int       `json:"round_no"`
	UserID       int64     `json:"user_id"`
	HitCount     int       `json:"hit_count"`
	RewardAmount string    `json:"reward_amount"`
	CreatedAt    time.Time `json:"created_at"`
}

type adminActivityUpsertRequest struct {
	Type          types.ActivityType        `json:"type"`
	Title         string                    `json:"title"`
	Description   string                    `json:"description"`
	CoverURL      string                    `json:"cover_url"`
	StartsAt      time.Time                 `json:"starts_at"`
	EndsAt        time.Time                 `json:"ends_at"`
	RedPacketRain types.RedPacketRainConfig `json:"red_packet_rain"`
}

func (r adminActivityUpsertRequest) toServiceInput(id int64, createdBy int64) (activityservice.UpsertActivityInput, error) {
	return activityservice.UpsertActivityInput{
		ID:            id,
		Type:          r.Type,
		Title:         strings.TrimSpace(r.Title),
		Description:   strings.TrimSpace(r.Description),
		CoverURL:      strings.TrimSpace(r.CoverURL),
		StartsAt:      r.StartsAt.UTC(),
		EndsAt:        r.EndsAt.UTC(),
		CreatedBy:     createdBy,
		RedPacketRain: r.RedPacketRain,
	}, nil
}

func userActivityDetailFromService(detail activityservice.ActivityDetail) userActivityDetail {
	item := userActivityItem{
		ID:          detail.Activity.ID,
		Type:        detail.Activity.Type,
		Title:       detail.Activity.Title,
		Description: detail.Activity.Description,
		CoverURL:    detail.Activity.CoverURL,
		Status:      detail.EffectiveStatus,
		StartsAt:    detail.Activity.StartsAt,
		EndsAt:      detail.Activity.EndsAt,
		Summary: userActivitySummary{
			TotalBudget:     detail.Config.TotalBudget,
			UserTotalReward: normalizeMoney(detail.ClaimSummary.UserActivityAmount),
		},
	}
	return userActivityDetail{
		userActivityItem: item,
		RedPacketRain: &userRedPacketRainConfig{
			RoundCount:           detail.Config.RoundCount,
			RoundDurationSeconds: detail.Config.RoundDurationSeconds,
			RoundIntervalSeconds: detail.Config.RoundIntervalSeconds,
			PerUserRoundCap:      detail.Config.PerUserRoundCap,
			PerUserTotalCap:      detail.Config.PerUserTotalCap,
		},
	}
}

func adminActivityItemFromSummary(summary types.ActivityAdminSummary) adminActivityItem {
	return adminActivityItem{
		ID:               summary.Activity.ID,
		Type:             summary.Activity.Type,
		Title:            summary.Activity.Title,
		Status:           activityservice.EffectiveActivityStatus(summary.Activity, time.Now().UTC()),
		StartsAt:         summary.Activity.StartsAt,
		EndsAt:           summary.Activity.EndsAt,
		TotalBudget:      normalizeMoney(summary.TotalBudget),
		IssuedAmount:     normalizeMoney(summary.IssuedAmount),
		ParticipantCount: summary.ParticipantCount,
	}
}

func adminActivityDetailFromService(detail activityservice.ActivityDetail) adminActivityDetail {
	item := adminActivityItem{
		ID:               detail.Activity.ID,
		Type:             detail.Activity.Type,
		Title:            detail.Activity.Title,
		Status:           detail.EffectiveStatus,
		StartsAt:         detail.Activity.StartsAt,
		EndsAt:           detail.Activity.EndsAt,
		TotalBudget:      detail.Config.TotalBudget,
		IssuedAmount:     normalizeMoney(detail.ClaimSummary.ActivityIssuedAmount),
		ParticipantCount: detail.ClaimSummary.ParticipantCount,
	}
	rounds := make([]adminRoundItem, 0, len(detail.Rounds))
	for _, round := range detail.Rounds {
		summary := detail.RoundSummaries[round.ID]
		rounds = append(rounds, adminRoundItem{
			ID:               round.ID,
			RoundNo:          round.RoundNo,
			Status:           round.Status,
			StartsAt:         round.StartsAt,
			EndsAt:           round.EndsAt,
			IssuedAmount:     normalizeMoney(summary.IssuedAmount),
			ParticipantCount: summary.ParticipantCount,
			ClaimCount:       summary.ClaimCount,
		})
	}
	return adminActivityDetail{
		adminActivityItem: item,
		Description:       detail.Activity.Description,
		CoverURL:          detail.Activity.CoverURL,
		RedPacketRain:     &detail.Config,
		Rounds:            rounds,
	}
}

func normalizeMoney(value string) string {
	if strings.TrimSpace(value) == "" {
		return "0.00000000"
	}
	return value
}
