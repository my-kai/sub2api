package turnlog

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Handler exposes administrator-only turn-log APIs.
type Handler struct{ service *Service }

// NewHandler creates the administrator HTTP adapter.
func NewHandler(runtime *Service) *Handler { return &Handler{service: runtime} }

// OAuthAccounts lists selectable OpenAI OAuth accounts without credentials.
func (h *Handler) OAuthAccounts(c *gin.Context) {
	accounts, err := h.service.OAuthAccounts(c.Request.Context())
	if err != nil {
		response.InternalError(c, "OpenAI OAuth账号查询失败")
		return
	}
	response.Success(c, accounts)
}

// Capture sends one manual diagnostic request and returns the raw upstream reply.
func (h *Handler) Capture(c *gin.Context) {
	var request struct {
		AccountID int64  `json:"account_id"`
		Model     string `json:"model"`
		ProxyID   *int64 `json:"proxy_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "捕捉请求无效")
		return
	}
	result, err := h.service.Capture(c.Request.Context(), c, request.AccountID, request.Model, request.ProxyID)
	if err != nil {
		if errors.Is(err, ErrInvalidCaptureRequest) {
			response.BadRequest(c, "捕捉参数无效")
			return
		}
		slog.Error("turn_log_manual_capture_failed", "account_id", request.AccountID, "model", request.Model, "proxy_id", request.ProxyID, "error", err)
		response.Error(c, http.StatusBadGateway, "OpenAI OAuth上游请求失败")
		return
	}
	response.Success(c, result)
}

// List returns paginated diagnostic rows.
func (h *Handler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		response.BadRequest(c, "page_size 超出上限")
		return
	}
	filter := TurnLogFilter{Page: page, PageSize: pageSize}
	if raw := strings.TrimSpace(c.Query("account_id")); raw != "" {
		value, ok := parsePositiveInt(raw)
		if !ok {
			response.BadRequest(c, "account_id 无效")
			return
		}
		filter.AccountID = &value
	}
	if raw := strings.TrimSpace(c.Query("status_code")); raw != "" {
		value, ok := parsePositiveInt(raw)
		if !ok || !isTurnStatus(int(value)) {
			response.BadRequest(c, "status_code 无效")
			return
		}
		status := int(value)
		filter.StatusCode = &status
	}
	result, err := h.service.List(c.Request.Context(), filter)
	if err != nil {
		response.InternalError(c, "turn记录查询失败")
		return
	}
	response.Success(c, result)
}

// Get returns one complete diagnostic row.
func (h *Handler) Get(c *gin.Context) {
	id, ok := parsePositiveInt(c.Param("id"))
	if !ok {
		response.BadRequest(c, "记录 ID 无效")
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err == sql.ErrNoRows {
		response.NotFound(c, "turn记录不存在")
		return
	}
	if err != nil {
		response.InternalError(c, "turn记录查询失败")
		return
	}
	response.Success(c, item)
}

// GetConfig returns the retention policy.
func (h *Handler) GetConfig(c *gin.Context) {
	config, err := h.service.Config(c.Request.Context())
	if err != nil {
		response.InternalError(c, "turn记录配置读取失败")
		return
	}
	response.Success(c, config)
}

// UpdateConfig updates retention days and triggers immediate cleanup.
func (h *Handler) UpdateConfig(c *gin.Context) {
	var request struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "turn记录配置请求无效")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "登录状态已失效")
		return
	}
	config, err := h.service.SaveConfig(c.Request.Context(), request.RetentionDays, subject.UserID)
	if err != nil {
		response.BadRequest(c, "保留天数必须在 1-3650 天之间")
		return
	}
	response.Success(c, config)
}
