package imagequeue

import (
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"
)

const (
	// DefaultPlatformConcurrency 是首次启动时的平台总生图并发。
	DefaultPlatformConcurrency = 2
	// DefaultUserConcurrency 是未配置覆盖时每个用户允许的默认并发。
	DefaultUserConcurrency = 1
	// DefaultRetentionDays 是终态任务默认保留天数，避免历史任务无限堆积。
	DefaultRetentionDays = 7
	// DefaultEnabled 是首次迁移后的生图功能默认状态，保持已有部署不被静默关闭。
	DefaultEnabled = true
	// Default1KImageUnitPrice 是 1K 分辨率的默认单张额度。
	Default1KImageUnitPrice = "0.13400"
	// Default2KImageUnitPrice 是 2K 分辨率的默认单张额度。
	Default2KImageUnitPrice = "0.26800"
	// Default4KImageUnitPrice 是 4K 分辨率的默认单张额度。
	Default4KImageUnitPrice = "0.40000"

	// JobStatusQueued 表示任务仍在持久化队列中等待 worker claim。
	JobStatusQueued JobStatus = "queued"
	// JobStatusRunning 表示任务已进入 chatgpt2api 调用阶段，此后不允许用户撤销。
	JobStatusRunning JobStatus = "running"
	// JobStatusCompleted 表示上游成功返回，并且图片链接已保存。
	JobStatusCompleted JobStatus = "completed"
	// JobStatusFailed 表示上游或本地执行失败，错误摘要已保存。
	JobStatusFailed JobStatus = "failed"
	// JobStatusCanceled 表示用户在 queued 阶段主动撤销了任务。
	JobStatusCanceled JobStatus = "canceled"

	// ChargeStatusNone 表示任务不需要余额扣款，兼容旧的免费生图路径。
	ChargeStatusNone ChargeStatus = "none"
	// ChargeStatusPending 表示任务已记录预计金额，正在等待余额扣减结果。
	ChargeStatusPending ChargeStatus = "pending"
	// ChargeStatusSuccess 表示任务余额已经扣减成功，可以进入正常队列生命周期。
	ChargeStatusSuccess ChargeStatus = "success"
	// ChargeStatusFailed 表示扣减余额失败，任务不会继续进入上游生图调用。
	ChargeStatusFailed ChargeStatus = "failed"
	// ChargeStatusRefunded 表示已扣减金额在撤销或执行失败后退款成功。
	ChargeStatusRefunded ChargeStatus = "refunded"
	// ChargeStatusRefundFailed 表示退款调用失败，需要后续人工排查或补偿。
	ChargeStatusRefundFailed ChargeStatus = "refund_failed"

	// GenerationModeGenerate 表示任务直接调用文生图接口。
	GenerationModeGenerate GenerationMode = "generate"
	// GenerationModeEdit 表示任务使用 Session 当前图片调用图片编辑接口。
	GenerationModeEdit GenerationMode = "edit"
)

var (
	// ErrInvalidInput 表示创建任务或保存配置时请求体不满足业务约束。
	ErrInvalidInput = errors.New("image queue input is invalid")
	// ErrBalanceNotConfigured 保留旧错误类型兼容；custom 生图当前不接主仓余额核心。
	ErrBalanceNotConfigured = errors.New("image queue balance client is not configured")
	// ErrBalanceChargeFailed 保留旧错误类型兼容；custom 生图当前不会主动扣减余额。
	ErrBalanceChargeFailed = errors.New("image generation balance charge failed")
	// ErrBalanceRefundFailed 保留旧错误类型兼容；custom 生图当前不会主动退款。
	ErrBalanceRefundFailed = errors.New("image generation balance refund failed")
	// ErrJobNotFound 表示指定任务不存在或当前用户不可见。
	ErrJobNotFound = errors.New("image generation job not found")
	// ErrSessionNotFound 表示指定 Session 不存在、已删除或当前用户不可见。
	ErrSessionNotFound = errors.New("image generation session not found")
	// ErrCancelNotAllowed 表示任务已经进入运行或终态，不能再撤销。
	ErrCancelNotAllowed = errors.New("image generation job cannot be canceled")
	// ErrRetryNotAllowed 表示任务不是失败终态，不能作为重试来源。
	ErrRetryNotAllowed = errors.New("image generation job cannot be retried")
	// ErrForbidden 表示当前用户无权访问指定任务或管理配置。
	ErrForbidden = errors.New("image queue access is forbidden")
	// ErrDisabled 表示管理员已关闭用户侧生图创建入口。
	ErrDisabled = errors.New("image generation is disabled")
)

// JobStatus 是生图任务的持久化状态机。
type JobStatus string

// ChargeStatus 是生图任务余额占用的持久化生命周期状态。
type ChargeStatus string

// GenerationMode 是任务进入 worker 后选择的上游执行模式。
type GenerationMode string

// PageRequest 是列表接口统一使用的分页输入。
type PageRequest struct {
	Page     int `form:"page" json:"page"`
	PageSize int `form:"page_size" json:"page_size"`
}

// PageResult 是分页响应的通用结构。
//
// Items 必须由调用方传入非 nil slice，保证 JSON 响应符合 backend/AGENTS.md 的数组约定。
type PageResult[T any] struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
	Pages    int   `json:"pages"`
	Items    []T   `json:"items"`
}

// ImageReference 指向某个已完成任务中的单张可作为编辑来源的图片。
type ImageReference struct {
	TaskID     int64
	ImageIndex int
}

// Session 保存一组连续生图任务的上下文。
//
// CurrentImageTaskID 和 CurrentImageIndex 指向本 Session 中后续编辑任务默认使用的图片。
type Session struct {
	ID                 int64      `json:"id"`
	UserID             int64      `json:"user_id,omitempty"`
	Username           string     `json:"username,omitempty"`
	Email              string     `json:"email,omitempty"`
	Title              string     `json:"title"`
	TitleCustomized    bool       `json:"title_customized,omitempty"`
	CurrentImageTaskID int64      `json:"current_image_task_id,omitempty"`
	CurrentImageIndex  *int       `json:"current_image_index,omitempty"`
	LastTaskID         int64      `json:"last_task_id,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
}

// CreateSessionInput 是用户创建 Session 的请求体。
type CreateSessionInput struct {
	Title string `json:"title"`
}

// UpdateSessionInput 是用户重命名 Session 的请求体。
type UpdateSessionInput struct {
	Title string `json:"title"`
}

// SetCurrentImageInput 是用户手动指定 Session 当前图片的请求体。
type SetCurrentImageInput struct {
	TaskID     int64 `json:"task_id"`
	ImageIndex int   `json:"image_index"`
}

// MyImage 是“我的图片”分页接口返回的单张图片。
type MyImage struct {
	TaskID        int64     `json:"task_id"`
	ImageIndex    int       `json:"image_index"`
	URL           string    `json:"url"`
	CreatedAt     time.Time `json:"created_at"`
	Prompt        string    `json:"prompt,omitempty"`
	GalleryItemID int64     `json:"gallery_item_id,omitempty"`
	InGallery     bool      `json:"in_gallery"`
}

// Config 保存管理员控制的平台并发与保留策略。
type Config struct {
	Enabled                bool           `json:"enabled"`
	PlatformConcurrency    int            `json:"platform_concurrency"`
	DefaultUserConcurrency int            `json:"default_user_concurrency"`
	RetentionDays          int            `json:"retention_days"`
	UnitPrices             UnitPrice      `json:"unit_prices"`
	ChatGPT2API            UpstreamConfig `json:"chatgpt2api"`
	UpdatedByUserID        int64          `json:"updated_by_user_id,omitempty"`
	UpdatedAt              time.Time      `json:"updated_at"`
}

// ConfigInput 是管理员保存全局并发配置的请求体。
type ConfigInput struct {
	Enabled                *bool               `json:"enabled"`
	PlatformConcurrency    int                 `json:"platform_concurrency"`
	DefaultUserConcurrency int                 `json:"default_user_concurrency"`
	RetentionDays          int                 `json:"retention_days"`
	UnitPrices             UnitPriceInput      `json:"unit_prices"`
	ChatGPT2API            UpstreamConfigInput `json:"chatgpt2api"`
}

// PublicStatus 是普通用户可读取的生图公开状态。
type PublicStatus struct {
	Enabled bool `json:"enabled"`
}

// UnitPrice 保存每种图片分辨率的单张额度。
//
// 金额以十进制字符串传输和保存，避免前后端用 float 造成配置值展示漂移。
type UnitPrice struct {
	OneK  string `json:"one_k"`
	TwoK  string `json:"two_k"`
	FourK string `json:"four_k"`
}

// UnitPriceInput 是管理员保存单张额度配置时的请求结构。
type UnitPriceInput struct {
	OneK  string `json:"one_k"`
	TwoK  string `json:"two_k"`
	FourK string `json:"four_k"`
}

// UpstreamConfig 保存 chatgpt2api 运行期上游配置。
//
// AuthKey 只在服务端进程内传递，JSON 序列化时永远不输出，避免管理页读取配置时泄露密钥。
type UpstreamConfig struct {
	BaseURL           string `json:"base_url"`
	AuthKey           string `json:"-"`
	AuthKeyConfigured bool   `json:"auth_key_configured"`
}

// UpstreamConfigInput 是管理员保存 chatgpt2api 配置时的请求结构。
//
// AuthKey 留空且 ClearAuthKey 为 false 时表示保留旧密钥；该语义让管理页无需回显敏感值。
type UpstreamConfigInput struct {
	BaseURL      string `json:"base_url"`
	AuthKey      string `json:"auth_key"`
	ClearAuthKey bool   `json:"clear_auth_key"`
}

// PriceQuote 是用户侧按当前参数计算出来的图片额度预览。
type PriceQuote struct {
	Model      string `json:"model"`
	Resolution string `json:"resolution"`
	Count      int    `json:"count"`
	UnitPrice  string `json:"unit_price"`
	TotalPrice string `json:"total_price"`
	Currency   string `json:"currency"`
}

// PriceQuoteInput 是用户侧查询图片额度预览的参数。
type PriceQuoteInput struct {
	Model      string `form:"model" json:"model"`
	Resolution string `form:"resolution" json:"resolution"`
	Size       string `form:"size" json:"size"`
	N          int    `form:"n" json:"n"`
}

// UserLimit 保存某个用户的并发覆盖值和用户资料快照。
type UserLimit struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username,omitempty"`
	Email       string    `json:"email,omitempty"`
	Concurrency int       `json:"concurrency"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserLimitInput 是管理员新增或编辑用户并发覆盖的请求体。
type UserLimitInput struct {
	Concurrency int `json:"concurrency"`
}

// UserLimitSnapshot 是保存覆盖时同步记录的用户展示信息。
//
// 快照由后端通过主仓用户读取接口注入，前端不再让管理员手动填写用户名或邮箱，
// 避免“备注”与真实用户资料不一致后误导后续排查。
type UserLimitSnapshot struct {
	Username string
	Email    string
}

// Job 保存单次生图请求的完整服务端状态。
//
// Result 只在 completed 后写入，内容只保留 chatgpt2api 返回的图片链接和元数据。
// 不保存 b64_json，避免 Postgres JSONB 和缓存被大图片正文撑大。
type Job struct {
	ID       int64     `json:"id"`
	UserID   int64     `json:"user_id"`
	Username string    `json:"username,omitempty"`
	Email    string    `json:"email,omitempty"`
	Status   JobStatus `json:"status"`
	// SessionID 将任务绑定到服务端持久化 Session；旧数据可能为空。
	SessionID int64 `json:"session_id,omitempty"`
	// GenerationMode 决定 worker 使用文生图还是图片编辑接口，旧数据默认按 generate 处理。
	GenerationMode GenerationMode `json:"generation_mode"`
	// SourceImageTaskID/SourceImageIndex 记录 edit 任务创建时复制的来源图片引用，避免执行时被 Session 当前图片变化影响。
	SourceImageTaskID int64 `json:"source_image_task_id,omitempty"`
	SourceImageIndex  *int  `json:"source_image_index,omitempty"`
	// SourceImageBytes/SourceImageFilename/SourceImageContentType 保存本次请求上传的编辑来源图。
	// 队列任务异步执行，不能依赖 HTTP 请求生命周期里的 multipart reader。
	SourceImageBytes       []byte `json:"-"`
	SourceImageFilename    string `json:"-"`
	SourceImageContentType string `json:"-"`
	Model                  string `json:"model"`
	Prompt                 string `json:"prompt"`
	N                      int    `json:"n"`
	Quality                string `json:"quality,omitempty"`
	Size                   string `json:"size,omitempty"`
	PublishToGallery       bool   `json:"publish_to_gallery"`
	// ChargeAmount 保存本任务真实扣减的额度字符串，避免金额经 float 转换后发生精度漂移。
	ChargeAmount string `json:"charge_amount"`
	// ChargeStatus 记录扣款与退款生命周期，和任务执行状态分离，方便排查余额补偿。
	ChargeStatus ChargeStatus `json:"charge_status"`
	// BalanceIdempotencyKey 保存任务级余额生命周期主键，具体扣款/退款请求可在此基础上派生。
	BalanceIdempotencyKey string `json:"balance_idempotency_key,omitempty"`
	// ChargeMessage 只保存扣款或退款失败摘要，避免覆盖上游生图错误。
	ChargeMessage string                               `json:"charge_message,omitempty"`
	QueuePosition int                                  `json:"queue_position,omitempty"`
	Result        *chatgpt2api.ImageGenerationResponse `json:"result,omitempty"`
	ErrorMessage  string                               `json:"error_message,omitempty"`
	CreatedAt     time.Time                            `json:"created_at"`
	StartedAt     *time.Time                           `json:"started_at,omitempty"`
	FinishedAt    *time.Time                           `json:"finished_at,omitempty"`
}

// CreateJobInput 是用户侧创建队列任务的请求体。
type CreateJobInput struct {
	SessionID              int64  `json:"session_id"`
	Model                  string `json:"model"`
	Prompt                 string `json:"prompt"`
	N                      int    `json:"n"`
	Quality                string `json:"quality,omitempty"`
	Size                   string `json:"size,omitempty"`
	PublishToGallery       bool   `json:"publish_to_gallery"`
	SourceImageBytes       []byte `json:"-"`
	SourceImageFilename    string `json:"-"`
	SourceImageContentType string `json:"-"`
}

// ClaimedJob 是 worker 原子 claim 后拿到的执行载荷。
type ClaimedJob struct {
	Job     Job
	Request chatgpt2api.ImageGenerationRequest
}

// IsTerminal 判断任务是否已经离开队列/执行状态。
func IsTerminal(status JobStatus) bool {
	return status == JobStatusCompleted || status == JobStatusFailed || status == JobStatusCanceled
}

// UserIdentity 从已校验的主仓 JWT 用户资料中提取队列归属字段。
func UserIdentity(user runtime.UserProfile) (int64, string, string) {
	return user.ID, user.Username, user.Email
}
