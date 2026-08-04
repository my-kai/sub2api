package aigatewayadmintransfer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const (
	// maxSourceEmailLength 限制来源邮箱长度，避免认证查询接收无界输入。
	maxSourceEmailLength = 320
	// maxSourcePasswordLength 限制密码请求字段长度，密码本身不做裁剪或持久化。
	maxSourcePasswordLength = 1024
)

// Service 协调来源账户认证、TOTP 校验、普通余额查询和原子扣款。
type Service struct {
	store       *Store
	userRepo    service.UserRepository
	totpService *service.TotpService
}

// NewService 创建来源直连转入服务；缺失用户仓储或 TOTP 服务时不允许启动半成品接口。
func NewService(store *Store, userRepo service.UserRepository, totpService *service.TotpService) (*Service, error) {
	if store == nil || userRepo == nil || totpService == nil {
		return nil, errors.New("ai-gateway admin transfer service dependencies are required")
	}
	return &Service{
		store:       store,
		userRepo:    userRepo,
		totpService: totpService,
	}, nil
}

// Balance 完成来源认证后读取普通余额；需要 TOTP 时不返回金额，避免绕过二次验证披露余额。
func (s *Service) Balance(ctx context.Context, input Credentials) (BalanceResult, error) {
	user, requiresTOTP, err := s.authenticate(ctx, input)
	if err != nil {
		return BalanceResult{}, err
	}
	if requiresTOTP {
		return BalanceResult{RequiresTOTP: true}, nil
	}
	balance, err := s.store.LoadRegularBalance(ctx, user.ID)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return BalanceResult{}, ErrInvalidCredentials
		}
		return BalanceResult{}, fmt.Errorf("load source balance: %w", err)
	}
	if balance == "" {
		return BalanceResult{}, fmt.Errorf("source regular balance is empty: %w", ErrServiceUnavailable)
	}
	return BalanceResult{AvailableBalanceUSD: balance}, nil
}

// Debit 再次完成来源认证后，以稳定转入 ID 原子扣减来源普通余额。
func (s *Service) Debit(ctx context.Context, input DebitInput) (Transfer, error) {
	if !validTransferID(input.TransferID) || normalizeAmount(input.AmountUSD) == "" {
		return Transfer{}, ErrInvalidInput
	}
	user, requiresTOTP, err := s.authenticate(ctx, input.Credentials)
	if err != nil {
		return Transfer{}, err
	}
	if requiresTOTP {
		return Transfer{}, ErrTotpRequired
	}
	transfer, err := s.store.CreateAndDebit(ctx, Transfer{
		TransferID: input.TransferID,
		UserID:     user.ID,
		AmountUSD:  input.AmountUSD,
	})
	if err != nil {
		return Transfer{}, err
	}
	return transfer, nil
}

// Status 返回来源账本中的最小恢复状态，不需要也不接受来源账户凭据。
func (s *Service) Status(ctx context.Context, transferID string) (StatusResult, error) {
	if !validTransferID(transferID) {
		return StatusResult{}, ErrInvalidInput
	}
	transfer, found, err := s.store.FindByTransferID(ctx, transferID)
	if err != nil {
		return StatusResult{}, err
	}
	if !found {
		return StatusResult{TransferID: transferID, Status: transferStatusNotFound}, nil
	}
	return StatusResult{
		TransferID: transfer.TransferID,
		AmountUSD:  transfer.AmountUSD,
		Status:     transfer.Status,
	}, nil
}

// authenticate 校验来源邮箱、密码和按账户启用状态要求的 TOTP，不产生登录会话或 Token。
func (s *Service) authenticate(ctx context.Context, input Credentials) (*service.User, bool, error) {
	email, ok := normalizeEmail(input.Email)
	if !ok || !validPassword(input.Password) || !validTOTPCode(input.TOTPCode) {
		return nil, false, ErrInvalidInput
	}
	if s == nil || s.userRepo == nil || s.totpService == nil {
		return nil, false, fmt.Errorf("source authentication dependencies are unavailable: %w", ErrServiceUnavailable)
	}
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			return nil, false, ErrInvalidCredentials
		}
		return nil, false, fmt.Errorf("load ai-gateway source account: %w", err)
	}
	if user == nil || !user.IsActive() || !user.CheckPassword(input.Password) {
		return nil, false, ErrInvalidCredentials
	}
	if !user.TotpEnabled {
		return user, false, nil
	}
	if input.TOTPCode == "" {
		return user, true, nil
	}
	if err := s.totpService.VerifyCode(ctx, user.ID, input.TOTPCode); err != nil {
		return nil, false, ErrInvalidTotp
	}
	return user, false, nil
}

// normalizeEmail 规范来源邮箱查询键，拒绝控制字符和超长输入。
func normalizeEmail(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || len(normalized) > maxSourceEmailLength || strings.ContainsAny(normalized, "\r\n") {
		return "", false
	}
	return normalized, true
}

// validPassword 仅验证密码字段是否可安全接收，保留原文本供 bcrypt 比较。
func validPassword(value string) bool {
	return value != "" && len(value) <= maxSourcePasswordLength
}

// validTOTPCode 验证可选 TOTP 字段的固定六码格式，空值由调用方决定是否允许。
func validTOTPCode(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != 6 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
