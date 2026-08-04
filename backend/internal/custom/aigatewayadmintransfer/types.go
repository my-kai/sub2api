package aigatewayadmintransfer

import "time"

const (
	// transferStatusDebited 表示来源普通余额已完成原子扣减，等待目标系统入账或恢复查询。
	transferStatusDebited = "debited"
	// transferStatusNotFound 表示来源不存在该稳定转入 ID，目标不得据此重复发起扣款。
	transferStatusNotFound = "not_found"
)

// Credentials 描述 ai-gateway 服务端转发的来源账户验证信息，只允许存在于当前请求内存中。
type Credentials struct {
	Email    string
	Password string
	TOTPCode string
}

// BalanceResult 描述完成来源认证后可安全返回的普通余额结果。
type BalanceResult struct {
	RequiresTOTP        bool   `json:"requires_totp"`
	AvailableBalanceUSD string `json:"available_balance_usd,omitempty"`
}

// DebitInput 描述一次稳定且可幂等的来源普通余额扣款请求。
type DebitInput struct {
	TransferID string
	Credentials
	AmountUSD string
}

// Transfer 保存来源扣款记录；来源用户 ID 永远不返回给 ai-gateway 或浏览器。
type Transfer struct {
	TransferID string    `json:"transfer_id"`
	UserID     int64     `json:"-"`
	AmountUSD  string    `json:"amount_usd"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// StatusResult 是来源恢复查询的最小响应，不携带用户资料、余额或凭据。
type StatusResult struct {
	TransferID string `json:"transfer_id"`
	AmountUSD  string `json:"amount_usd,omitempty"`
	Status     string `json:"status"`
}
