package imagegenhandoff

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	exchangeSecretHeader = "X-Image-Gen-Exchange-Secret"
	loginExpiredMessage  = "登录失效，请重新进入生图页面"
	unavailableMessage   = "生图服务暂不可用，请稍后重试"
)

// Config contains only the sub2api-ex side of the image-gen handoff contract.
type Config struct {
	BaseURL        string
	ExchangeSecret string
	CodeTTLSeconds int
}

// UserReader is the minimal user-service surface needed for identity snapshots.
type UserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

// Handler owns login-code generation and service-to-service exchange.
type Handler struct {
	cfg    Config
	store  CodeStore
	users  UserReader
	now    func() time.Time
	logger *slog.Logger
}

// NewHandler creates a handoff handler with normalized configuration.
func NewHandler(cfg Config, store CodeStore, users UserReader) *Handler {
	return &Handler{
		cfg:    normalizeConfig(cfg),
		store:  store,
		users:  users,
		now:    time.Now,
		logger: slog.Default(),
	}
}

// NewMemoryStoreForConfig builds the first-phase single-instance code store.
func NewMemoryStoreForConfig(cfg Config) *MemoryCodeStore {
	return NewMemoryCodeStore(time.Duration(cfg.CodeTTLSeconds) * time.Second)
}

// LoginCode issues a one-time code for the current sub2api-ex user.
func (h *Handler) LoginCode(c *gin.Context) {
	if h == nil || h.store == nil || h.users == nil ||
		strings.TrimSpace(h.cfg.BaseURL) == "" || strings.TrimSpace(h.cfg.ExchangeSecret) == "" {
		response.Error(c, http.StatusServiceUnavailable, unavailableMessage)
		return
	}

	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "登录失效，请重新登录")
		return
	}

	user, err := h.users.GetByID(c.Request.Context(), subject.UserID)
	if err != nil || user == nil || !user.IsActive() {
		response.Unauthorized(c, "登录失效，请重新登录")
		return
	}

	record, err := h.store.Create(c.Request.Context(), buildIdentity(user, h.now().UTC()))
	if err != nil {
		h.logError("create image-gen login code failed", err)
		response.InternalError(c, unavailableMessage)
		return
	}

	redirectURL, err := buildRedirectURL(h.cfg.BaseURL, record.Code)
	if err != nil {
		h.logError("build image-gen redirect url failed", err)
		response.Error(c, http.StatusServiceUnavailable, unavailableMessage)
		return
	}

	response.Success(c, gin.H{
		"redirect_url": redirectURL,
		"expires_at":   record.ExpiresAt,
	})
}

// Exchange consumes a one-time code for the trusted image-gen backend.
func (h *Handler) Exchange(c *gin.Context) {
	if h == nil || h.store == nil {
		writeExchangeError(c, http.StatusServiceUnavailable, "service_unavailable", unavailableMessage)
		return
	}
	if strings.TrimSpace(h.cfg.ExchangeSecret) == "" {
		writeExchangeError(c, http.StatusServiceUnavailable, "service_unavailable", unavailableMessage)
		return
	}
	if !constantTimeEqual(c.GetHeader(exchangeSecretHeader), h.cfg.ExchangeSecret) {
		writeExchangeError(c, http.StatusUnauthorized, "login_expired", loginExpiredMessage)
		return
	}

	var req exchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		writeExchangeError(c, http.StatusUnauthorized, "login_expired", loginExpiredMessage)
		return
	}

	record, err := h.store.Consume(c.Request.Context(), req.Code)
	if err != nil {
		if !errors.Is(err, ErrCodeExpired) {
			h.logError("consume image-gen login code failed", err)
		}
		writeExchangeError(c, http.StatusUnauthorized, "login_expired", loginExpiredMessage)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":       record.Identity,
		"expires_at": record.ExpiresAt,
	})
}

type exchangeRequest struct {
	Code string `json:"code"`
}

func normalizeConfig(cfg Config) Config {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.ExchangeSecret = strings.TrimSpace(cfg.ExchangeSecret)
	return cfg
}

func buildIdentity(user *service.User, issuedAt time.Time) Identity {
	return Identity{
		ExternalUserID: strconv.FormatInt(user.ID, 10),
		Username:       strings.TrimSpace(user.Username),
		Email:          strings.TrimSpace(user.Email),
		Role:           strings.TrimSpace(user.Role),
		IsAdmin:        user.IsAdmin(),
		IssuedAt:       issuedAt,
	}
}

func buildRedirectURL(baseURL, code string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrInvalidConfig
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrInvalidConfig
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/auth/callback"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	values := parsed.Query()
	values.Set("code", code)
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func constantTimeEqual(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == "" || want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func writeExchangeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func (h *Handler) logError(message string, err error) {
	if h != nil && h.logger != nil && err != nil {
		// Do not log one-time codes or shared secrets; the message is enough for ops correlation.
		h.logger.Warn(message, "err", err)
	}
}
