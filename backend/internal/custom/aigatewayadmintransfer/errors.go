// Package aigatewayadmintransfer 提供 ai-gateway 通过管理员 API Key 触发的来源余额查询与原子扣款能力。
package aigatewayadmintransfer

import "errors"

var (
	// ErrInvalidInput 表示请求字段、金额或稳定转入 ID 不符合领域约束。
	ErrInvalidInput = errors.New("invalid ai-gateway admin transfer input")
	// ErrInvalidCredentials 统一隐藏来源账户不存在、不可用或密码不正确的差异。
	ErrInvalidCredentials = errors.New("invalid ai-gateway source credentials")
	// ErrTotpRequired 表示密码已通过但来源账户还必须提供 TOTP 验证码。
	ErrTotpRequired = errors.New("ai-gateway source totp is required")
	// ErrInvalidTotp 表示来源账户提交的 TOTP 验证码未通过校验。
	ErrInvalidTotp = errors.New("invalid ai-gateway source totp")
	// ErrInsufficientBalance 表示来源普通余额不足以完成本次扣款。
	ErrInsufficientBalance = errors.New("insufficient ai-gateway source balance")
	// ErrTransferConflict 表示同一稳定转入 ID 被用于不同账户或金额。
	ErrTransferConflict = errors.New("ai-gateway source transfer conflicts with existing record")
	// ErrServiceUnavailable 表示来源用户、TOTP 或数据库依赖不可用于安全完成本次操作。
	ErrServiceUnavailable = errors.New("ai-gateway source transfer service is unavailable")
)
