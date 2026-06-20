package callbackauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	invalidCallbackMessage = "回跳地址无效"
	deniedCallbackMessage  = "回跳地址未被允许"
	loginExpiredMessage    = "登录失效，请重新登录"
	unavailableMessage     = "授权服务暂不可用"
)

// UserReader is the minimal user-service surface needed by callback exchange.
type UserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

// AllowlistReader is implemented by SettingService without exposing the
// allowlist through public settings.
type AllowlistReader interface {
	GetCallbackAuthAllowedDomains(ctx context.Context) []string
}

// Handler owns callback consent, durable authorization and one-time code exchange.
type Handler struct {
	store     *Store
	users     UserReader
	allowlist AllowlistReader
	now       func() time.Time
	logger    *slog.Logger
}

// NewHandler wires callback authorization dependencies.
func NewHandler(store *Store, users UserReader, allowlist AllowlistReader) *Handler {
	return &Handler{
		store:     store,
		users:     users,
		allowlist: allowlist,
		now:       time.Now,
		logger:    slog.Default(),
	}
}

// Info returns callback target details and whether the current user already
// granted durable consent for the callback domain.
func (h *Handler) Info(c *gin.Context) {
	user, ok := h.currentActiveUser(c)
	if !ok {
		return
	}
	target, ok := h.allowedCallbackTarget(c, c.Query("callback"))
	if !ok {
		return
	}

	authorized, err := h.store.IsAuthorized(c.Request.Context(), user.ID, target.Domain)
	if err != nil {
		h.logError("query callback authorization failed", err)
		response.InternalError(c, unavailableMessage)
		return
	}
	response.Success(c, CallbackInfo{
		Callback:   target.URL,
		Domain:     target.Domain,
		Authorized: authorized,
	})
}

// Authorize records user consent for the callback domain and issues a one-time
// code that is appended to the callback URL.
func (h *Handler) Authorize(c *gin.Context) {
	user, ok := h.currentActiveUser(c)
	if !ok {
		return
	}

	var req authorizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, invalidCallbackMessage)
		return
	}
	target, ok := h.allowedCallbackTarget(c, req.Callback)
	if !ok {
		return
	}

	if err := h.store.UpsertAuthorization(c.Request.Context(), user.ID, target.Domain); err != nil {
		h.logError("upsert callback authorization failed", err)
		response.InternalError(c, unavailableMessage)
		return
	}
	code, record, err := h.store.CreateCode(c.Request.Context(), user.ID, target.URL, target.Domain)
	if err != nil {
		h.logError("create callback auth code failed", err)
		response.InternalError(c, unavailableMessage)
		return
	}
	redirectURL, err := buildRedirectURL(target.URL, code)
	if err != nil {
		response.BadRequest(c, invalidCallbackMessage)
		return
	}
	response.Success(c, AuthorizeResponse{
		RedirectURL: redirectURL,
		Code:        code,
		ExpiresAt:   record.ExpiresAt,
	})
}

// Exchange consumes a one-time code and returns a small user snapshot to the
// callback system. The code itself is the bearer credential and can be used once.
func (h *Handler) Exchange(c *gin.Context) {
	if h == nil || h.store == nil || h.users == nil {
		response.Error(c, http.StatusServiceUnavailable, unavailableMessage)
		return
	}

	var req exchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		response.Unauthorized(c, loginExpiredMessage)
		return
	}

	record, err := h.store.ConsumeCode(c.Request.Context(), req.Code)
	if err != nil {
		if !errors.Is(err, ErrCodeExpired) {
			h.logError("consume callback auth code failed", err)
		}
		response.Unauthorized(c, loginExpiredMessage)
		return
	}

	user, err := h.users.GetByID(c.Request.Context(), record.UserID)
	if err != nil || user == nil || !user.IsActive() {
		response.Unauthorized(c, loginExpiredMessage)
		return
	}

	response.Success(c, ExchangeResponse{
		User:           buildUserSnapshot(user, h.now().UTC()),
		CallbackDomain: record.CallbackDomain,
		CallbackURL:    record.CallbackURL,
		ExpiresAt:      record.ExpiresAt,
	})
}

type authorizeRequest struct {
	Callback string `json:"callback"`
}

type exchangeRequest struct {
	Code string `json:"code"`
}

func (h *Handler) currentActiveUser(c *gin.Context) (*service.User, bool) {
	if h == nil || h.store == nil || h.users == nil {
		response.Error(c, http.StatusServiceUnavailable, unavailableMessage)
		return nil, false
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, loginExpiredMessage)
		return nil, false
	}
	user, err := h.users.GetByID(c.Request.Context(), subject.UserID)
	if err != nil || user == nil || !user.IsActive() {
		response.Unauthorized(c, loginExpiredMessage)
		return nil, false
	}
	return user, true
}

func (h *Handler) allowedCallbackTarget(c *gin.Context, raw string) (callbackTarget, bool) {
	target, err := NormalizeCallback(raw)
	if err != nil {
		response.BadRequest(c, invalidCallbackMessage)
		return callbackTarget{}, false
	}
	allowlist := []string{}
	if h != nil && h.allowlist != nil {
		allowlist = h.allowlist.GetCallbackAuthAllowedDomains(c.Request.Context())
	}
	if !service.IsCallbackAuthDomainAllowed(target.Domain, allowlist) {
		response.Forbidden(c, deniedCallbackMessage)
		return callbackTarget{}, false
	}
	return target, true
}

func buildUserSnapshot(user *service.User, issuedAt time.Time) UserSnapshot {
	return UserSnapshot{
		ID:             user.ID,
		ExternalUserID: strconv.FormatInt(user.ID, 10),
		Username:       strings.TrimSpace(user.Username),
		Email:          strings.TrimSpace(user.Email),
		Role:           strings.TrimSpace(user.Role),
		IsAdmin:        user.IsAdmin(),
		IssuedAt:       issuedAt,
	}
}

func (h *Handler) logError(message string, err error) {
	if h != nil && h.logger != nil && err != nil {
		// Codes and callback URLs are intentionally omitted from logs.
		h.logger.Warn(message, "err", err)
	}
}
