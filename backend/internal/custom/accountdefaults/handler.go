package accountdefaults

import (
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Handler exposes administrator-only account-defaults APIs.
type Handler struct{ service *Service }

// NewHandler creates the administrator HTTP adapter.
func NewHandler(svc *Service) *Handler { return &Handler{service: svc} }

// updateRequest mirrors Config with tolerant JSON fields (all optional on input).
type updateRequest struct {
	ModelsByPlatform map[string][]string `json:"models_by_platform"`
	ProxyMode        string              `json:"proxy_mode"`
	ProxyFixedIDs    []int64             `json:"proxy_fixed_ids"`
	AllowLocalEgress bool                `json:"allow_local_egress"`
}

// Get returns the persisted defaults configuration.
func (h *Handler) Get(c *gin.Context) {
	cfg, err := h.service.Config(c.Request.Context())
	if err != nil {
		response.InternalError(c, "账号默认配置读取失败")
		return
	}
	response.Success(c, cfg)
}

// Update validates and persists the defaults configuration.
func (h *Handler) Update(c *gin.Context) {
	var request updateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "账号默认配置请求无效")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "登录状态已失效")
		return
	}
	cfg := Config{
		ModelsByPlatform: request.ModelsByPlatform,
		ProxyMode:        ProxyMode(strings.TrimSpace(request.ProxyMode)),
		ProxyFixedIDs:    request.ProxyFixedIDs,
		AllowLocalEgress: request.AllowLocalEgress,
	}
	saved, err := h.service.SaveConfig(c.Request.Context(), cfg, subject.UserID)
	if err != nil {
		if errors.Is(err, ErrInvalidConfig) {
			response.BadRequest(c, "账号默认配置无效：固定模式需至少选择一个代理或允许本地直连")
			return
		}
		response.InternalError(c, "账号默认配置保存失败")
		return
	}
	response.Success(c, saved)
}

// Resolve returns the form-ready defaults (random proxy already drawn).
func (h *Handler) Resolve(c *gin.Context) {
	resolved, err := h.service.resolve(c.Request.Context())
	if err != nil {
		response.InternalError(c, "账号默认配置解析失败")
		return
	}
	response.Success(c, resolved)
}
