package runtime

import "errors"

var (
	// ErrUnauthorized 供 custom handler 将主仓鉴权失败统一映射为 401。
	ErrUnauthorized = errors.New("imagegen auth is unauthorized")
	// ErrForbidden 供 custom handler 将主仓管理员权限失败统一映射为 403。
	ErrForbidden = errors.New("imagegen auth is forbidden")
)

// UserProfile 是 custom 生图模块从主仓 JWT 上下文需要读取的最小用户快照。
//
// Username 和 Email 只用于任务、会话和用户限制展示快照；主仓解析失败时可以留空，
// 但 ID 必须来自已校验 JWT 上下文，避免重新引入外部 profile client。
type UserProfile struct {
	ID          int64
	Username    string
	Email       string
	Role        string
	Concurrency int
}

// AdminUserSummary 是管理员配置用户并发覆盖时展示的候选用户安全子集。
type AdminUserSummary struct {
	ID       int64  `json:"id"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role,omitempty"`
}
