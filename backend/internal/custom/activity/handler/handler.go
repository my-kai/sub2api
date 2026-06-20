package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	activityservice "github.com/Wei-Shaw/sub2api/internal/custom/activity/service"
	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

const redPacketRainWSReadLimitBytes = 16 * 1024

// Handler exposes custom activity HTTP APIs while keeping business rules in service.
type Handler struct {
	service *activityservice.Service
}

// NewHandler creates a custom activity handler.
func NewHandler(service *activityservice.Service) *Handler {
	return &Handler{service: service}
}

// ListUserActivities returns activities visible to the current user.
func (h *Handler) ListUserActivities(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "登录失效，请重新登录")
		return
	}
	items, _, err := h.service.ListVisibleActivities(c.Request.Context(), parsePage(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	out := make([]userActivityItem, 0, len(items))
	for _, item := range items {
		cfg, err := h.service.ActivityStore().GetRedPacketRainConfig(c.Request.Context(), item.ID)
		if err != nil {
			h.writeError(c, err)
			return
		}
		summary, err := h.service.ActivityStore().ClaimSummary(c.Request.Context(), item.ID, 0, subject.UserID)
		if err != nil {
			h.writeError(c, err)
			return
		}
		out = append(out, userActivityItem{
			ID:          item.ID,
			Type:        item.Type,
			Title:       item.Title,
			Description: item.Description,
			CoverURL:    item.CoverURL,
			Status:      activityservice.EffectiveActivityStatus(item, time.Now().UTC()),
			StartsAt:    item.StartsAt,
			EndsAt:      item.EndsAt,
			Summary: userActivitySummary{
				TotalBudget:     cfg.TotalBudget,
				UserTotalReward: normalizeMoney(summary.UserActivityAmount),
			},
		})
	}
	response.Success(c, gin.H{"items": out})
}

// GetUserActivity returns one user-facing activity detail.
func (h *Handler) GetUserActivity(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "登录失效，请重新登录")
		return
	}
	activityID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	detail, err := h.service.GetAdminActivityDetail(c.Request.Context(), activityID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	userSummary, err := h.service.ActivityStore().ClaimSummary(c.Request.Context(), activityID, 0, subject.UserID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	detail.ClaimSummary.UserActivityAmount = userSummary.UserActivityAmount
	response.Success(c, userActivityDetailFromService(detail))
}

// GetRedPacketRainState returns the current or next round state.
func (h *Handler) GetRedPacketRainState(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "登录失效，请重新登录")
		return
	}
	activityID, parsed := parseIDParam(c, "id")
	if !parsed {
		return
	}
	state, err := h.service.GetRedPacketRainState(c.Request.Context(), activityID, subject.UserID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, state)
}

// IssueRedPacketRainWSTicket returns a short-lived ticket for the claim WebSocket.
func (h *Handler) IssueRedPacketRainWSTicket(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "登录失效，请重新登录")
		return
	}
	activityID, parsed := parseIDParam(c, "id")
	if !parsed {
		return
	}
	var req wsTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	result, err := h.service.IssueRedPacketRainWSTicket(c.Request.Context(), activityservice.WSTicketInput{
		ActivityID:        activityID,
		RoundID:           req.RoundID,
		UserID:            subject.UserID,
		DeviceFingerprint: req.DeviceFingerprint,
		ClientNonce:       req.ClientNonce,
		ClientIP:          c.ClientIP(),
		UserAgent:         c.GetHeader("User-Agent"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

// ServeRedPacketRainWS upgrades one ticket-bound red packet rain claim session.
func (h *Handler) ServeRedPacketRainWS(c *gin.Context) {
	activityID, parsed := parseIDParam(c, "id")
	if !parsed {
		return
	}
	challenge, err := h.service.OpenRedPacketRainWSSession(c.Request.Context(), activityservice.WSChallengeInput{
		ActivityID: activityID,
		Ticket:     c.Query("ticket"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}

	conn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{
		CompressionMode: coderws.CompressionDisabled,
		Subprotocols:    []string{"sub2api-activity"},
	})
	if err != nil {
		return
	}
	defer func() {
		h.service.CloseRedPacketRainWSSession(context.WithoutCancel(c.Request.Context()), challenge.SessionID)
		_ = conn.CloseNow()
	}()
	conn.SetReadLimit(redPacketRainWSReadLimitBytes)

	if err := writeWSJSON(c.Request.Context(), conn, wsChallengeMessage{
		Type:        "challenge",
		SessionID:   challenge.SessionID,
		ServerNonce: challenge.ServerNonce,
		Challenge:   challenge.Challenge,
		ExpiresAt:   challenge.ExpiresAt,
		RoundID:     challenge.RoundID,
		RoundEndsAt: challenge.RoundEndsAt,
	}); err != nil {
		return
	}
	h.readRedPacketRainWS(c.Request.Context(), conn, challenge.UserID, challenge)
}

// ClaimRedPacketRain settles one red packet rain claim.
func (h *Handler) ClaimRedPacketRain(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "登录失效，请重新登录")
		return
	}
	activityID, parsed := parseIDParam(c, "id")
	if !parsed {
		return
	}
	var req claimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	result, err := h.service.ClaimRedPacketRain(c.Request.Context(), activityservice.ClaimInput{
		ActivityID:     activityID,
		RoundID:        req.RoundID,
		UserID:         subject.UserID,
		HitCount:       req.HitCount,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) readRedPacketRainWS(ctx context.Context, conn *coderws.Conn, userID int64, challenge activityservice.WSChallengeResult) {
	for {
		messageType, payload, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if messageType != coderws.MessageText {
			_ = writeWSJSON(ctx, conn, wsErrorMessage{Type: "error", Message: "领取失败"})
			return
		}
		var base wsMessage
		if err := json.Unmarshal(payload, &base); err != nil {
			_ = writeWSJSON(ctx, conn, wsErrorMessage{Type: "error", Message: "领取失败"})
			return
		}
		switch strings.TrimSpace(base.Type) {
		case "ping":
			_ = writeWSJSON(ctx, conn, gin.H{"type": "state"})
		case "claim":
			h.handleRedPacketRainWSClaim(ctx, conn, userID, challenge, payload)
		default:
			_ = writeWSJSON(ctx, conn, wsErrorMessage{Type: "error", Message: "领取失败"})
		}
	}
}

func (h *Handler) handleRedPacketRainWSClaim(ctx context.Context, conn *coderws.Conn, userID int64, challenge activityservice.WSChallengeResult, payload []byte) {
	var req wsClaimMessage
	if err := json.Unmarshal(payload, &req); err != nil {
		_ = writeWSJSON(ctx, conn, wsErrorMessage{Type: "error", Message: "领取失败"})
		return
	}
	if strings.TrimSpace(req.SessionID) != challenge.SessionID || req.RoundID != challenge.RoundID {
		_ = writeWSJSON(ctx, conn, wsClaimResultMessage{
			Type: "claim_result",
			Data: types.ClaimResult{
				ClaimID:      0,
				ActivityID:   0,
				RoundID:      req.RoundID,
				HitCount:     0,
				RewardAmount: "0.00000000",
				Credited:     false,
				Duplicate:    false,
				Message:      activityservice.ErrorMessage(types.ErrRedPacketRainSecurityRejected),
			},
		})
		return
	}
	result, err := h.service.ClaimRedPacketRainFromWS(ctx, userID, activityservice.WSClaimEnvelope{
		SessionID:      req.SessionID,
		RoundID:        req.RoundID,
		IdempotencyKey: req.IdempotencyKey,
		Nonce:          req.Nonce,
		Ciphertext:     req.Ciphertext,
		Signature:      req.Signature,
	}, challenge.Key)
	if err != nil {
		result = types.ClaimResult{
			ClaimID:      0,
			ActivityID:   0,
			RoundID:      req.RoundID,
			HitCount:     0,
			RewardAmount: "0.00000000",
			Credited:     false,
			Duplicate:    false,
			Message:      activityservice.ErrorMessage(err),
		}
	}
	_ = writeWSJSON(ctx, conn, wsClaimResultMessage{Type: "claim_result", Data: result})
}

// ListAdminActivities returns admin activity summaries.
func (h *Handler) ListAdminActivities(c *gin.Context) {
	items, total, err := h.service.ListAdminSummaries(c.Request.Context(), parsePage(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	out := make([]adminActivityItem, 0, len(items))
	for _, item := range items {
		out = append(out, adminActivityItemFromSummary(item))
	}
	response.Success(c, gin.H{"items": out, "total": total})
}

// CreateAdminActivity creates a red packet rain activity.
func (h *Handler) CreateAdminActivity(c *gin.Context) {
	subject, _ := middleware2.GetAuthSubjectFromContext(c)
	var req adminActivityUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	input, err := req.toServiceInput(0, subject.UserID)
	if err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	detail, err := h.service.CreateRedPacketRainActivity(c.Request.Context(), input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Created(c, adminActivityDetailFromService(detail))
}

// GetAdminActivity returns admin activity detail.
func (h *Handler) GetAdminActivity(c *gin.Context) {
	activityID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	detail, err := h.service.GetAdminActivityDetail(c.Request.Context(), activityID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, adminActivityDetailFromService(detail))
}

// UpdateAdminActivity updates an activity before it starts.
func (h *Handler) UpdateAdminActivity(c *gin.Context) {
	activityID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req adminActivityUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	input, err := req.toServiceInput(activityID, 0)
	if err != nil {
		response.BadRequest(c, "请求参数无效")
		return
	}
	detail, err := h.service.UpdateRedPacketRainActivity(c.Request.Context(), input)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, adminActivityDetailFromService(detail))
}

// EndAdminActivity ends an activity early.
func (h *Handler) EndAdminActivity(c *gin.Context) {
	activityID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	activity, err := h.service.EndActivity(c.Request.Context(), activityID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, gin.H{"id": activity.ID, "status": activity.Status, "message": "活动已结束"})
}

// OfflineAdminActivity hides an activity from users.
func (h *Handler) OfflineAdminActivity(c *gin.Context) {
	activityID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	activity, err := h.service.OfflineActivity(c.Request.Context(), activityID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, gin.H{"id": activity.ID, "status": activity.Status, "message": "活动已下架"})
}

// ListAdminClaims returns activity claim records for audit views.
func (h *Handler) ListAdminClaims(c *gin.Context) {
	activityID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	page := parsePage(c)
	claims, total, err := h.service.ActivityStore().ListClaims(c.Request.Context(), activityID, page)
	if err != nil {
		h.writeError(c, err)
		return
	}
	rounds, _ := h.service.ActivityStore().ListRounds(c.Request.Context(), activityID)
	roundNos := map[int64]int{}
	for _, round := range rounds {
		roundNos[round.ID] = round.RoundNo
	}
	out := make([]adminClaimItem, 0, len(claims))
	for _, claim := range claims {
		out = append(out, adminClaimItem{
			ID:           claim.ID,
			RoundNo:      roundNos[claim.RoundID],
			UserID:       claim.UserID,
			HitCount:     claim.HitCount,
			RewardAmount: claim.RewardAmount,
			CreatedAt:    claim.CreatedAt,
		})
	}
	response.Success(c, gin.H{"items": out, "total": total})
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, types.ErrNotFound):
		response.NotFound(c, "活动不存在")
	case errors.Is(err, types.ErrInvalidInput):
		response.BadRequest(c, "请求参数无效")
	case errors.Is(err, types.ErrRedPacketRainSecurityRejected):
		response.Error(c, http.StatusTooManyRequests, "领取失败")
	case errors.Is(err, types.ErrActivityNotStarted), errors.Is(err, types.ErrActivityEnded),
		errors.Is(err, types.ErrActivityOffline), errors.Is(err, types.ErrRoundNotStarted),
		errors.Is(err, types.ErrRoundEnded), errors.Is(err, types.ErrUserRoundCapReached),
		errors.Is(err, types.ErrUserTotalCapReached), errors.Is(err, types.ErrBudgetExhausted):
		response.Error(c, http.StatusConflict, activityservice.ErrorMessage(err))
	default:
		response.InternalError(c, "操作失败")
	}
}

func parseIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param(name)), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "请求参数无效")
		return 0, false
	}
	return id, true
}

func parsePage(c *gin.Context) types.PageRequest {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", c.DefaultQuery("limit", "20")))
	return types.PageRequest{Page: page, PageSize: pageSize}
}

func writeWSJSON(ctx context.Context, conn *coderws.Conn, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return conn.Write(ctx, coderws.MessageText, payload)
}
