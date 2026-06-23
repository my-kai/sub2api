package oauthapp

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

const (
	invalidApplicationMessage = "应用信息无效"
	unavailableMessage        = "授权服务暂不可用"
	loginExpiredMessage       = "登录失效，请重新登录"
)

// Handler 对外提供管理员应用管理和 OAuth 授权接口。
type Handler struct {
	service *Service
}

// NewHandler 创建 OAuth 应用处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ListApplications 返回不包含密钥的管理员应用列表。
func (h *Handler) ListApplications(c *gin.Context) {
	apps, err := h.service.ListApplications(c.Request.Context())
	if err != nil {
		response.InternalError(c, unavailableMessage)
		return
	}
	response.Success(c, apps)
}

// CreateApplication 创建客户端，并只在本次响应返回明文密钥。
func (h *Handler) CreateApplication(c *gin.Context) {
	var req createApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidApplicationMessage)
		return
	}
	result, err := h.service.CreateApplication(c.Request.Context(), req)
	if err != nil {
		writeApplicationError(c, err)
		return
	}
	response.Created(c, result)
}

// UpdateApplication 更新客户端元信息、白名单和启用状态。
func (h *Handler) UpdateApplication(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req updateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidApplicationMessage)
		return
	}
	result, err := h.service.UpdateApplication(c.Request.Context(), id, req)
	if err != nil {
		writeApplicationError(c, err)
		return
	}
	response.Success(c, result)
}

// ResetSecret 轮换客户端密钥，并只在本次响应返回新明文密钥。
func (h *Handler) ResetSecret(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	result, err := h.service.ResetSecret(c.Request.Context(), id)
	if err != nil {
		writeApplicationError(c, err)
		return
	}
	response.Success(c, result)
}

// DeleteApplication 软删除客户端应用。
func (h *Handler) DeleteApplication(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteApplication(c.Request.Context(), id); err != nil {
		writeApplicationError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// AuthorizeInfo 校验 OAuth authorize 参数，并返回授权确认页信息。
func (h *Handler) AuthorizeInfo(c *gin.Context) {
	if !ensureAuthenticated(c) {
		return
	}
	info, err := h.service.GetAuthorizeInfo(
		c.Request.Context(),
		c.Query("response_type"),
		c.Query("client_id"),
		c.Query("redirect_uri"),
		c.Query("state"),
	)
	if err != nil {
		writePublicOAuthError(c, err)
		return
	}
	response.Success(c, info)
}

// RedirectAuthorizePageIfHTML 允许浏览器从 API 形态的 authorize 地址进入授权页。
// 非 HTML 请求继续进入 JWT 保护的预览接口，避免未登录时暴露应用信息。
func (h *Handler) RedirectAuthorizePageIfHTML(c *gin.Context) {
	if !wantsHTML(c) {
		c.Next()
		return
	}
	c.Redirect(http.StatusFound, buildAuthorizePageURL(c.Request.URL.Query()))
	c.Abort()
}

// AuthorizeConfirm 记录用户授权确认，并返回携带 code 和 state 的回调地址。
func (h *Handler) AuthorizeConfirm(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, loginExpiredMessage)
		return
	}
	var req AuthorizeConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "应用授权请求无效")
		return
	}
	result, err := h.service.CreateAuthorizationCode(c.Request.Context(), subject.UserID, req)
	if err != nil {
		writePublicOAuthError(c, err)
		return
	}
	response.Success(c, result)
}

// Token 使用授权码换取站内正常 token。
func (h *Handler) Token(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		response.BadRequest(c, "应用授权请求无效")
		return
	}
	result, err := h.service.ExchangeCode(
		c.Request.Context(),
		c.PostForm("grant_type"),
		c.PostForm("code"),
		c.PostForm("client_id"),
		c.PostForm("client_secret"),
		c.PostForm("redirect_uri"),
	)
	if err != nil {
		writePublicOAuthError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, invalidApplicationMessage)
		return 0, false
	}
	return id, true
}

func ensureAuthenticated(c *gin.Context) bool {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, loginExpiredMessage)
		return false
	}
	return true
}

func writeApplicationError(c *gin.Context, err error) {
	status, message := classifyPublicError(err)
	if status >= 500 {
		response.InternalError(c, message)
		return
	}
	response.Error(c, status, message)
}

func writePublicOAuthError(c *gin.Context, err error) {
	status, message := classifyPublicError(err)
	response.Error(c, status, message)
}

func wantsHTML(c *gin.Context) bool {
	accept := strings.ToLower(c.GetHeader("Accept"))
	return strings.Contains(accept, "text/html")
}

func buildAuthorizePageURL(query url.Values) string {
	values := url.Values{}
	for _, key := range []string{"response_type", "client_id", "redirect_uri", "state"} {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			values.Set(key, value)
		}
	}
	if encoded := values.Encode(); encoded != "" {
		return "/auth/oauth/authorize?" + encoded
	}
	return "/auth/oauth/authorize"
}
