package aigatewayadmintransfer

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

const (
	// maxRequestBodyBytes 限制携带来源凭据的请求体，防止专用认证端点成为内存放大入口。
	maxRequestBodyBytes int64 = 4096
)

// Handler 提供仅允许管理员 API Key 调用的来源余额和扣款 HTTP 接口。
type Handler struct {
	service *Service
}

// NewHandler 创建来源直连转入 handler；服务缺失时返回 nil，薄路由层将拒绝注册接口。
func NewHandler(service *Service) *Handler {
	if service == nil {
		return nil
	}
	return &Handler{service: service}
}

// Balance 完成来源账户认证后返回普通余额或需要 TOTP 的最小状态。
func (h *Handler) Balance(c *gin.Context) {
	var request credentialsRequest
	if !bindRequest(c, &request) {
		return
	}
	result, err := h.service.Balance(c.Request.Context(), request.credentials())
	if err != nil {
		writeTransferError(c, "balance", err)
		return
	}
	response.Success(c, result)
}

// Debit 完成来源账户认证后原子扣减普通余额，并返回稳定来源转入记录。
func (h *Handler) Debit(c *gin.Context) {
	var request debitRequest
	if !bindRequest(c, &request) {
		return
	}
	transfer, err := h.service.Debit(c.Request.Context(), DebitInput{
		TransferID:  request.TransferID,
		Credentials: request.credentials(),
		AmountUSD:   request.AmountUSD,
	})
	if err != nil {
		writeTransferError(c, "debit", err)
		return
	}
	response.Success(c, transfer)
}

// Status 返回来源账本的恢复状态，不接收或记录来源账户凭据。
func (h *Handler) Status(c *gin.Context) {
	var request statusRequest
	if !bindRequest(c, &request) {
		return
	}
	result, err := h.service.Status(c.Request.Context(), request.TransferID)
	if err != nil {
		writeTransferError(c, "status", err)
		return
	}
	response.Success(c, result)
}

// bindRequest 统一限制敏感请求体大小并返回不包含字段内容的通用格式错误。
func bindRequest(c *gin.Context, target any) bool {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
	if err := c.ShouldBindJSON(target); err != nil {
		response.ErrorWithDetails(c, http.StatusBadRequest, "请求格式不正确", "AI_GATEWAY_ADMIN_TRANSFER_INVALID_REQUEST", nil)
		return false
	}
	return true
}

// writeTransferError 将领域错误映射为稳定业务响应；未知服务异常仅记录安全错误上下文，避免回传或记录来源凭据与内部错误细节。
func writeTransferError(c *gin.Context, operation string, err error) {
	status := http.StatusServiceUnavailable
	reason := "AI_GATEWAY_ADMIN_TRANSFER_UNAVAILABLE"
	message := "暂时无法处理余额转入"
	switch {
	case errors.Is(err, ErrInvalidInput):
		status, reason, message = http.StatusBadRequest, "AI_GATEWAY_ADMIN_TRANSFER_INVALID_REQUEST", "请求参数不正确"
	case errors.Is(err, ErrInvalidCredentials):
		status, reason, message = http.StatusUnauthorized, "AI_GATEWAY_ADMIN_TRANSFER_INVALID_CREDENTIALS", "邮箱或密码不正确"
	case errors.Is(err, ErrTotpRequired):
		status, reason, message = http.StatusUnauthorized, "AI_GATEWAY_ADMIN_TRANSFER_TOTP_REQUIRED", "需要二次验证"
	case errors.Is(err, ErrInvalidTotp):
		status, reason, message = http.StatusUnauthorized, "AI_GATEWAY_ADMIN_TRANSFER_INVALID_TOTP", "验证码不正确"
	case errors.Is(err, ErrInsufficientBalance):
		status, reason, message = http.StatusConflict, "AI_GATEWAY_ADMIN_TRANSFER_INSUFFICIENT_BALANCE", "普通余额不足"
	case errors.Is(err, ErrTransferConflict):
		status, reason, message = http.StatusConflict, "AI_GATEWAY_ADMIN_TRANSFER_CONFLICT", "转入请求冲突"
	}
	if status == http.StatusServiceUnavailable {
		// 只记录服务端可排查的错误上下文，来源账户凭据和原始请求内容不得进入日志。
		slog.Default().Error(
			"ai-gateway admin transfer request failed",
			"operation", operation,
			"error", err,
		)
	}
	response.ErrorWithDetails(c, status, message, reason, nil)
}

// credentialsRequest 描述来源账户验证字段，密码和 TOTP 只停留在当前请求结构内。
type credentialsRequest struct {
	Email    string `json:"email" binding:"required,max=320"`
	Password string `json:"password" binding:"required,max=1024"`
	TOTPCode string `json:"totp_code" binding:"omitempty,len=6,numeric"`
}

// credentials 转换 handler 输入为领域输入，避免在领域层依赖 Gin 的绑定结构。
func (r credentialsRequest) credentials() Credentials {
	return Credentials{
		Email:    r.Email,
		Password: r.Password,
		TOTPCode: r.TOTPCode,
	}
}

// debitRequest 描述稳定来源扣款所需的账户验证、金额和转入 ID。
type debitRequest struct {
	credentialsRequest
	TransferID string `json:"transfer_id" binding:"required,max=128"`
	AmountUSD  string `json:"amount_usd" binding:"required,max=30"`
}

// statusRequest 描述目标恢复流程读取来源扣款状态所需的唯一字段。
type statusRequest struct {
	TransferID string `json:"transfer_id" binding:"required,max=128"`
}
