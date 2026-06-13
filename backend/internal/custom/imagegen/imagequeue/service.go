package imagequeue

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"
)

const (
	// DefaultImageModel 是 custom 生图当前唯一开放的模型。
	DefaultImageModel       = "gpt-image-2"
	defaultImageModel       = DefaultImageModel
	defaultImageSessionName = "新会话"
	// maxQueuedImageCount 是单个队列任务允许保存的总图数；worker 会按上游单次限制拆批执行。
	maxQueuedImageCount   = 100
	maxUploadedImageBytes = 64 << 20
	maxPromptLength       = 8000
	maxSessionTitleLen    = 80
)

// ImageClient 是 worker 调用 chatgpt2api 所需的窄接口。
type ImageClient interface {
	GenerateImage(ctx context.Context, input chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error)
	EditImage(ctx context.Context, input chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error)
}

// ChatGPT2APIRuntimeConfig 返回 worker/handler 调用上游时需要的最新配置。
func (s *Service) ChatGPT2APIRuntimeConfig(ctx context.Context) (chatgpt2api.RuntimeConfig, error) {
	if s == nil || s.store == nil {
		return chatgpt2api.RuntimeConfig{}, nil
	}
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return chatgpt2api.RuntimeConfig{}, err
	}
	return chatgpt2api.NewRuntimeConfig(cfg.ChatGPT2API.BaseURL, cfg.ChatGPT2API.AuthKey)
}

type taskSourceSnapshot struct {
	Mode             GenerationMode
	TaskID           int64
	Index            *int
	ImageBytes       []byte
	ImageFilename    string
	ImageContentType string
}

// Service 封装生图队列的用户侧和管理员侧业务规则。
type Service struct {
	store    *Store
	eventHub *TaskEventHub
	locks    *userLockPool
	now      func() time.Time
}

// NewService 创建生图队列服务。
func NewService(store *Store) *Service {
	return &Service{
		store: store,
		locks: newUserLockPool(),
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// WithTaskEventHub 安装任务事件中心，供 HTTP SSE 连接实时刷新任务快照。
func (s *Service) WithTaskEventHub(hub *TaskEventHub) *Service {
	s.eventHub = hub
	return s
}

// WithNow 覆盖当前时间来源，主要用于持久化清理和状态机测试。
func (s *Service) WithNow(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// CreateSession 为当前用户创建一个持久化生图 Session。
func (s *Service) CreateSession(ctx context.Context, user runtime.UserProfile, input CreateSessionInput) (Session, error) {
	if user.ID <= 0 {
		return Session{}, fmt.Errorf("%w: user id is required", ErrInvalidInput)
	}
	title, err := normalizeSessionTitle(input.Title, false)
	if err != nil {
		if strings.TrimSpace(input.Title) != "" {
			return Session{}, err
		}
		return s.createSessionWithDefaultTitle(ctx, user)
	}
	return s.createSessionWithTitle(ctx, user, title, true)
}

// createSessionWithDefaultTitle 在用户维度串行生成默认标题，避免并发新建时拿到相同序号。
func (s *Service) createSessionWithDefaultTitle(ctx context.Context, user runtime.UserProfile) (Session, error) {
	unlock := s.locks.lock(user.ID)
	defer unlock()

	titles, err := s.store.ListSessionTitles(ctx, user.ID)
	if err != nil {
		return Session{}, err
	}
	return s.createSessionWithTitle(ctx, user, defaultSessionTitleFromTitles(titles), false)
}

// createSessionWithTitle 保存已归一化的 Session 标题，并固定用户快照，方便后台排查归属。
func (s *Service) createSessionWithTitle(ctx context.Context, user runtime.UserProfile, title string, customized bool) (Session, error) {
	userID, username, email := UserIdentity(user)
	return s.store.CreateSession(ctx, Session{
		UserID:          userID,
		Username:        username,
		Email:           email,
		Title:           title,
		TitleCustomized: customized,
		CreatedAt:       s.now(),
		UpdatedAt:       s.now(),
	})
}

// Sessions 返回当前用户最近活跃的 Session 列表；空数据固定返回 []。
func (s *Service) Sessions(ctx context.Context, user runtime.UserProfile) ([]Session, error) {
	if user.ID <= 0 {
		return nil, fmt.Errorf("%w: user id is required", ErrInvalidInput)
	}
	items, err := s.store.ListSessions(ctx, user.ID, 100)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []Session{}
	}
	return items, nil
}

// UpdateSession 修改当前用户自己的 Session 标题。
func (s *Service) UpdateSession(ctx context.Context, user runtime.UserProfile, sessionID int64, input UpdateSessionInput) (Session, error) {
	if user.ID <= 0 || sessionID <= 0 {
		return Session{}, fmt.Errorf("%w: session id is required", ErrInvalidInput)
	}
	title, err := normalizeSessionTitle(input.Title, false)
	if err != nil {
		return Session{}, err
	}
	return s.store.UpdateSessionTitle(ctx, user.ID, sessionID, title, s.now())
}

// DeleteSession 软删除当前用户自己的 Session，不删除其下历史任务。
func (s *Service) DeleteSession(ctx context.Context, user runtime.UserProfile, sessionID int64) error {
	if user.ID <= 0 || sessionID <= 0 {
		return fmt.Errorf("%w: session id is required", ErrInvalidInput)
	}
	return s.store.SoftDeleteSession(ctx, user.ID, sessionID, s.now())
}

// SetCurrentImage 校验任务结果后，把指定图片设为 Session 后续编辑的来源图片。
func (s *Service) SetCurrentImage(ctx context.Context, user runtime.UserProfile, sessionID int64, input SetCurrentImageInput) (Session, error) {
	if user.ID <= 0 || sessionID <= 0 {
		return Session{}, fmt.Errorf("%w: session id is required", ErrInvalidInput)
	}
	if input.TaskID <= 0 || input.ImageIndex < 0 {
		return Session{}, fmt.Errorf("%w: image reference is invalid", ErrInvalidInput)
	}
	if _, err := s.store.GetUserSession(ctx, user.ID, sessionID); err != nil {
		return Session{}, err
	}
	if err := s.validateImageReference(ctx, user.ID, sessionID, input.TaskID, input.ImageIndex); err != nil {
		return Session{}, err
	}
	return s.store.SetSessionCurrentImage(ctx, user.ID, sessionID, input.TaskID, input.ImageIndex, s.now())
}

// ResetCurrentImage 取消手动指定图片，并恢复到当前会话默认最新编辑图。
func (s *Service) ResetCurrentImage(ctx context.Context, user runtime.UserProfile, sessionID int64) (Session, error) {
	if user.ID <= 0 || sessionID <= 0 {
		return Session{}, fmt.Errorf("%w: session id is required", ErrInvalidInput)
	}
	if _, err := s.store.GetUserSession(ctx, user.ID, sessionID); err != nil {
		return Session{}, err
	}
	return s.store.ResetSessionCurrentImage(ctx, user.ID, sessionID, s.now())
}

// SessionTasks 返回当前用户指定 Session 的任务流分页，并补充 queued 任务排队位置。
func (s *Service) SessionTasks(ctx context.Context, user runtime.UserProfile, sessionID int64, page PageRequest) (PageResult[Job], error) {
	if user.ID <= 0 || sessionID <= 0 {
		return PageResult[Job]{Page: 1, PageSize: 20, Items: []Job{}}, fmt.Errorf("%w: session id is required", ErrInvalidInput)
	}
	if _, err := s.store.GetUserSession(ctx, user.ID, sessionID); err != nil {
		return PageResult[Job]{Page: 1, PageSize: 20, Items: []Job{}}, err
	}
	result, err := s.store.ListSessionJobs(ctx, user.ID, sessionID, page)
	if err != nil {
		return result, err
	}
	for index, job := range result.Items {
		if job.Status != JobStatusQueued {
			continue
		}
		withPosition, err := s.attachQueuePosition(ctx, job)
		if err != nil {
			return result, err
		}
		result.Items[index] = withPosition
	}
	if result.Items == nil {
		result.Items = []Job{}
	}
	return result, nil
}

// MyImages 返回当前用户已完成任务里的图片分页；每页数量由 store 统一限制到 20。
func (s *Service) MyImages(ctx context.Context, user runtime.UserProfile, page PageRequest) (PageResult[MyImage], error) {
	if user.ID <= 0 {
		return PageResult[MyImage]{Page: 1, PageSize: 20, Items: []MyImage{}}, fmt.Errorf("%w: user id is required", ErrInvalidInput)
	}
	result, err := s.store.ListMyImages(ctx, user.ID, page)
	if err != nil {
		return result, err
	}
	if result.Items == nil {
		result.Items = []MyImage{}
	}
	return result, nil
}

// CreateTask 校验用户输入后把请求保存为可调度 queued 任务。
func (s *Service) CreateTask(ctx context.Context, user runtime.UserProfile, input CreateJobInput) (Job, error) {
	return s.createTask(ctx, user, input, nil)
}

func (s *Service) createTask(ctx context.Context, user runtime.UserProfile, input CreateJobInput, retrySource *taskSourceSnapshot) (Job, error) {
	if user.ID <= 0 {
		return Job{}, fmt.Errorf("%w: user id is required", ErrInvalidInput)
	}
	normalized, err := normalizeCreateInput(input)
	if err != nil {
		return Job{}, err
	}
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return Job{}, err
	}
	if !cfg.Enabled {
		return Job{}, ErrDisabled
	}
	price, err := priceQuoteFromConfig(cfg, normalized.Model, resolutionFromSize(normalized.Size), normalized.N)
	if err != nil {
		return Job{}, err
	}

	unlock := s.locks.lock(user.ID)
	defer unlock()

	session, err := s.store.GetUserSession(ctx, user.ID, normalized.SessionID)
	if err != nil {
		return Job{}, err
	}

	generationMode := GenerationModeGenerate
	sourceImageTaskID := int64(0)
	var sourceImageIndex *int
	sourceImageBytes := normalized.SourceImageBytes
	sourceImageFilename := normalized.SourceImageFilename
	sourceImageContentType := normalized.SourceImageContentType
	if retrySource != nil {
		generationMode = retrySource.Mode
		if generationMode == "" {
			generationMode = GenerationModeGenerate
		}
		if generationMode == GenerationModeEdit {
			if len(retrySource.ImageBytes) > 0 {
				sourceImageBytes = retrySource.ImageBytes
				sourceImageFilename = retrySource.ImageFilename
				sourceImageContentType = retrySource.ImageContentType
			} else if retrySource.TaskID <= 0 || retrySource.Index == nil {
				return Job{}, fmt.Errorf("%w: source image is required", ErrInvalidInput)
			} else {
				if err := s.validateImageReference(ctx, user.ID, session.ID, retrySource.TaskID, *retrySource.Index); err != nil {
					return Job{}, err
				}
				sourceImageTaskID = retrySource.TaskID
				sourceImageIndex = copyInt(retrySource.Index)
			}
		}
		if generationMode != GenerationModeGenerate && generationMode != GenerationModeEdit {
			return Job{}, fmt.Errorf("%w: generation_mode is invalid", ErrInvalidInput)
		}
	} else if len(sourceImageBytes) > 0 {
		generationMode = GenerationModeEdit
	} else if session.CurrentImageTaskID > 0 && session.CurrentImageIndex != nil {
		if err := s.validateImageReference(ctx, user.ID, session.ID, session.CurrentImageTaskID, *session.CurrentImageIndex); err != nil {
			return Job{}, err
		}
		generationMode = GenerationModeEdit
		sourceImageTaskID = session.CurrentImageTaskID
		sourceImageIndex = copyInt(session.CurrentImageIndex)
	}

	userID, username, email := UserIdentity(user)
	job, err := s.store.CreateJob(ctx, Job{
		UserID:                 userID,
		Username:               username,
		Email:                  email,
		Status:                 JobStatusQueued,
		SessionID:              normalized.SessionID,
		GenerationMode:         generationMode,
		SourceImageTaskID:      sourceImageTaskID,
		SourceImageIndex:       sourceImageIndex,
		SourceImageBytes:       sourceImageBytes,
		SourceImageFilename:    sourceImageFilename,
		SourceImageContentType: sourceImageContentType,
		Model:                  normalized.Model,
		Prompt:                 normalized.Prompt,
		N:                      normalized.N,
		Quality:                normalized.Quality,
		Size:                   normalized.Size,
		PublishToGallery:       normalized.PublishToGallery,
		ChargeAmount:           price.TotalPrice,
		// custom 迁移阶段不接主仓余额核心；价格只做配置与预览快照，任务默认不写余额流水。
		ChargeStatus: ChargeStatusNone,
		CreatedAt:    s.now(),
	})
	if err != nil {
		return Job{}, err
	}
	if retrySource == nil {
		if err := s.autoNameSessionForFirstPrompt(ctx, session, job.ID, normalized.Prompt); err != nil {
			return Job{}, err
		}
	}
	result, err := s.attachQueuePosition(ctx, job)
	s.publishTaskEvent()
	return result, err
}

// CreateJob 保留给内部旧调用方；新 HTTP contract 使用 CreateTask。
func (s *Service) CreateJob(ctx context.Context, user runtime.UserProfile, input CreateJobInput) (Job, error) {
	return s.CreateTask(ctx, user, input)
}

// RetryTask 使用失败任务的原始参数创建一个新的 queued 任务。
//
// 失败任务可能已经触发退款或退款失败，直接把原任务改回 queued 会破坏余额幂等链路；
// 因此重试始终创建新任务，并重新走扣款、排队和 Session 当前图片快照逻辑。
func (s *Service) RetryTask(ctx context.Context, user runtime.UserProfile, id int64) (Job, error) {
	source, err := s.Task(ctx, user, id)
	if err != nil {
		return Job{}, err
	}
	if source.Status != JobStatusFailed {
		return Job{}, ErrRetryNotAllowed
	}
	return s.createTask(ctx, user, CreateJobInput{
		SessionID:        source.SessionID,
		Model:            source.Model,
		Prompt:           source.Prompt,
		N:                source.N,
		Quality:          source.Quality,
		Size:             source.Size,
		PublishToGallery: source.PublishToGallery,
	}, retrySourceSnapshotFromJob(source))
}

func retrySourceSnapshotFromJob(job Job) *taskSourceSnapshot {
	return &taskSourceSnapshot{
		Mode:             job.GenerationMode,
		TaskID:           job.SourceImageTaskID,
		Index:            copyInt(job.SourceImageIndex),
		ImageBytes:       job.SourceImageBytes,
		ImageFilename:    job.SourceImageFilename,
		ImageContentType: job.SourceImageContentType,
	}
}

// Job 返回单个任务，并按用户角色执行可见性校验。
func (s *Service) Job(ctx context.Context, user runtime.UserProfile, id int64) (Job, error) {
	job, err := s.store.GetJob(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if !canAccessJob(user, job) {
		return Job{}, ErrForbidden
	}
	return s.attachQueuePosition(ctx, job)
}

// Task 返回单个任务，并按用户角色执行可见性校验。
func (s *Service) Task(ctx context.Context, user runtime.UserProfile, id int64) (Job, error) {
	return s.Job(ctx, user, id)
}

// CancelJob 只允许用户或管理员撤销尚未进入运行态的 queued 任务。
func (s *Service) CancelJob(ctx context.Context, user runtime.UserProfile, id int64) (Job, error) {
	job, err := s.store.GetJob(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if !canAccessJob(user, job) {
		return Job{}, ErrForbidden
	}
	unlock := s.locks.lock(job.UserID)
	defer unlock()

	canceled, err := s.store.CancelQueuedJob(ctx, id, s.now())
	if err != nil {
		return Job{}, err
	}
	refunded, err := s.refundChargedJob(ctx, canceled)
	if err != nil {
		s.publishTaskEvent()
		return refunded, err
	}
	result, err := s.attachQueuePosition(ctx, refunded)
	s.publishTaskEvent()
	return result, err
}

// CancelTask 只允许用户或管理员撤销尚未进入运行态的 queued 任务。
func (s *Service) CancelTask(ctx context.Context, user runtime.UserProfile, id int64) (Job, error) {
	return s.CancelJob(ctx, user, id)
}

// Config 返回管理员配置。
func (s *Service) Config(ctx context.Context) (Config, error) {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	cfg.UnitPrices = withDefaultUnitPrices(cfg.UnitPrices)
	cfg.ChatGPT2API = sanitizeUpstreamConfig(cfg.ChatGPT2API)
	return cfg, nil
}

// PublicStatus 返回普通用户可读取的生图公开状态。
func (s *Service) PublicStatus(ctx context.Context) (PublicStatus, error) {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return PublicStatus{}, err
	}
	return PublicStatus{Enabled: cfg.Enabled}, nil
}

// UpdateConfig 校验并保存管理员配置。
func (s *Service) UpdateConfig(ctx context.Context, input ConfigInput, admin runtime.UserProfile) (Config, error) {
	current, err := s.store.GetConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	upstream := mergeUpstreamConfigInput(current.ChatGPT2API, input.ChatGPT2API)
	cfg := Config{
		Enabled:                configInputEnabled(input, current.Enabled),
		PlatformConcurrency:    input.PlatformConcurrency,
		DefaultUserConcurrency: input.DefaultUserConcurrency,
		RetentionDays:          input.RetentionDays,
		UnitPrices:             withDefaultUnitPrices(normalizeUnitPriceInput(input.UnitPrices)),
		ChatGPT2API:            upstream,
		UpdatedByUserID:        admin.ID,
		UpdatedAt:              s.now(),
	}
	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}
	if err := s.store.UpsertConfig(ctx, cfg); err != nil {
		return Config{}, err
	}
	cfg.ChatGPT2API = sanitizeUpstreamConfig(cfg.ChatGPT2API)
	return cfg, nil
}

// QuotePrice 按管理员配置返回当前分辨率和数量对应的图片额度预览。
func (s *Service) QuotePrice(ctx context.Context, input PriceQuoteInput) (PriceQuote, error) {
	normalized, err := normalizePriceQuoteInput(input)
	if err != nil {
		return PriceQuote{}, err
	}
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return PriceQuote{}, err
	}
	return priceQuoteFromConfig(cfg, normalized.Model, normalized.Resolution, normalized.N)
}

func priceQuoteFromConfig(cfg Config, model string, resolution string, n int) (PriceQuote, error) {
	if n < 1 || n > maxQueuedImageCount {
		return PriceQuote{}, fmt.Errorf("%w: n must be between 1 and %d", ErrInvalidInput, maxQueuedImageCount)
	}
	unitPrice := unitPriceForResolution(cfg.UnitPrices, resolution)
	totalPrice, err := multiplyDecimalByInt(unitPrice, n)
	if err != nil {
		return PriceQuote{}, fmt.Errorf("%w: unit price is invalid", ErrInvalidInput)
	}
	return PriceQuote{
		Model:      model,
		Resolution: resolution,
		Count:      n,
		UnitPrice:  unitPrice,
		TotalPrice: totalPrice,
		Currency:   "$",
	}, nil
}

func configInputEnabled(input ConfigInput, fallback bool) bool {
	if input.Enabled == nil {
		return fallback
	}
	return *input.Enabled
}

// UserLimits 返回所有用户并发覆盖；空数据固定返回 []。
func (s *Service) UserLimits(ctx context.Context) ([]UserLimit, error) {
	items, err := s.store.ListUserLimits(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []UserLimit{}
	}
	return items, nil
}

// UpsertUserLimit 保存某个用户的并发覆盖。
func (s *Service) UpsertUserLimit(ctx context.Context, userID int64, input UserLimitInput, snapshot UserLimitSnapshot) (UserLimit, error) {
	limit := UserLimit{
		UserID:      userID,
		Username:    strings.TrimSpace(snapshot.Username),
		Email:       strings.TrimSpace(snapshot.Email),
		Concurrency: input.Concurrency,
		UpdatedAt:   s.now(),
	}
	if err := validateUserLimit(limit); err != nil {
		return UserLimit{}, err
	}
	return s.store.UpsertUserLimit(ctx, limit)
}

// DeleteUserLimit 删除某个用户的并发覆盖。
func (s *Service) DeleteUserLimit(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return fmt.Errorf("%w: user id is required", ErrInvalidInput)
	}
	return s.store.DeleteUserLimit(ctx, userID)
}

// SubscribeTaskEvents 订阅任务变化事件；SSE 端用它触发当前 Session 快照刷新。
func (s *Service) SubscribeTaskEvents(ctx context.Context) (<-chan TaskEvent, func()) {
	if s.eventHub == nil {
		return NewTaskEventHub().Subscribe(ctx)
	}
	return s.eventHub.Subscribe(ctx)
}

// PublishTaskEvent 广播任务变化，worker 在 claim/complete/fail 等状态转换后调用。
func (s *Service) PublishTaskEvent() {
	s.publishTaskEvent()
}

// CleanupExpiredTerminalJobs 按当前配置清理旧终态任务。
func (s *Service) CleanupExpiredTerminalJobs(ctx context.Context) (int, error) {
	cfg, err := s.store.GetConfig(ctx)
	if err != nil {
		return 0, err
	}
	cutoff := s.now().Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour)
	return s.store.CleanupTerminalJobs(ctx, cutoff)
}

func (s *Service) attachQueuePosition(ctx context.Context, job Job) (Job, error) {
	position, err := s.store.QueuePosition(ctx, job.ID)
	if err != nil {
		return Job{}, err
	}
	job.QueuePosition = position
	return job, nil
}

func (s *Service) publishTaskEvent() {
	if s != nil && s.eventHub != nil {
		s.eventHub.Publish()
	}
}

func (s *Service) priceForNormalizedInput(ctx context.Context, model string, size string, n int) (PriceQuote, error) {
	return s.QuotePrice(ctx, PriceQuoteInput{Model: model, Size: size, N: n})
}

// RefundFailedJobIfCharged 在 worker 失败后归还已扣减余额，并持久化退款状态。
func (s *Service) RefundFailedJobIfCharged(ctx context.Context, job Job) (Job, error) {
	unlock := s.locks.lock(job.UserID)
	defer unlock()
	return s.refundChargedJob(ctx, job)
}

func (s *Service) refundChargedJob(ctx context.Context, job Job) (Job, error) {
	// custom 生图当前不接主仓余额流水，失败/取消不需要做余额补偿。
	return job, nil
}

func normalizeCreateInput(input CreateJobInput) (CreateJobInput, error) {
	if input.SessionID <= 0 {
		return CreateJobInput{}, fmt.Errorf("%w: session_id is required", ErrInvalidInput)
	}
	model := defaultImageModel
	count := input.N
	if count == 0 {
		count = 1
	}
	normalized := CreateJobInput{
		SessionID:              input.SessionID,
		Model:                  model,
		Prompt:                 strings.TrimSpace(input.Prompt),
		N:                      count,
		Quality:                normalizeQuality(input.Quality),
		Size:                   normalizeSize(input.Size),
		PublishToGallery:       input.PublishToGallery,
		SourceImageBytes:       input.SourceImageBytes,
		SourceImageFilename:    strings.TrimSpace(input.SourceImageFilename),
		SourceImageContentType: strings.TrimSpace(input.SourceImageContentType),
	}
	if normalized.Prompt == "" {
		return CreateJobInput{}, fmt.Errorf("%w: prompt is required", ErrInvalidInput)
	}
	if len(normalized.Prompt) > maxPromptLength {
		return CreateJobInput{}, fmt.Errorf("%w: prompt is too long", ErrInvalidInput)
	}
	if normalized.N < 1 || normalized.N > maxQueuedImageCount {
		return CreateJobInput{}, fmt.Errorf("%w: n must be between 1 and %d", ErrInvalidInput, maxQueuedImageCount)
	}
	if err := normalizeUploadedImageSource(&normalized); err != nil {
		return CreateJobInput{}, err
	}
	return normalized, nil
}

func normalizeUploadedImageSource(input *CreateJobInput) error {
	if input == nil || len(input.SourceImageBytes) == 0 {
		if input != nil {
			input.SourceImageFilename = ""
			input.SourceImageContentType = ""
		}
		return nil
	}
	if len(input.SourceImageBytes) > maxUploadedImageBytes {
		return fmt.Errorf("%w: image is too large", ErrInvalidInput)
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(input.SourceImageContentType, ";")[0]))
	if !strings.HasPrefix(contentType, "image/") {
		contentType = http.DetectContentType(input.SourceImageBytes)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return fmt.Errorf("%w: source image must be an image", ErrInvalidInput)
	}
	filename := filepath.Base(strings.TrimSpace(input.SourceImageFilename))
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		filename = "source.png"
	}
	input.SourceImageContentType = contentType
	input.SourceImageFilename = filename
	return nil
}

func normalizeSessionTitle(raw string, allowDefault bool) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" && allowDefault {
		title = defaultSessionTitleFromTitles(nil)
	}
	if title == "" {
		return "", fmt.Errorf("%w: session title is required", ErrInvalidInput)
	}
	if len([]rune(title)) > maxSessionTitleLen {
		return "", fmt.Errorf("%w: session title is too long", ErrInvalidInput)
	}
	return title, nil
}

func (s *Service) autoNameSessionForFirstPrompt(ctx context.Context, session Session, firstTaskID int64, prompt string) error {
	if session.TitleCustomized || session.LastTaskID > 0 {
		return nil
	}
	title := promptSessionTitle(prompt)
	if title == "" {
		return nil
	}
	if _, err := s.store.AutoNameSessionFromPrompt(ctx, session.UserID, session.ID, firstTaskID, title, s.now()); err != nil {
		return err
	}
	return nil
}

// promptSessionTitle 从首个提示词里取一句短摘要作为默认会话名，避免空白和换行撑坏会话列表。
func promptSessionTitle(prompt string) string {
	fields := strings.Fields(strings.TrimSpace(prompt))
	if len(fields) == 0 {
		return ""
	}
	title := strings.Join(fields, " ")
	runes := []rune(title)
	if len(runes) <= maxSessionTitleLen {
		return title
	}
	return string(runes[:maxSessionTitleLen-1]) + "…"
}

// defaultSessionTitleFromTitles 根据用户历史标题生成下一个默认会话名。
//
// 这里使用历史最大序号 + 1，而不是填补中间空缺，避免软删除或重命名后再次出现用户记忆中用过的标题。
func defaultSessionTitleFromTitles(titles []string) string {
	maxIndex := 0
	for _, title := range titles {
		index, ok := defaultSessionTitleIndex(title)
		if ok && index > maxIndex {
			maxIndex = index
		}
	}
	return fmt.Sprintf("%s %d", defaultImageSessionName, maxIndex+1)
}

// defaultSessionTitleIndex 只识别本服务生成过的默认标题，用户自定义标题不参与序号计算。
func defaultSessionTitleIndex(title string) (int, bool) {
	trimmed := strings.TrimSpace(title)
	if trimmed == defaultImageSessionName {
		return 1, true
	}
	suffix := strings.TrimSpace(strings.TrimPrefix(trimmed, defaultImageSessionName))
	if suffix == trimmed || suffix == "" {
		return 0, false
	}
	index, err := strconv.Atoi(suffix)
	if err != nil || index <= 0 {
		return 0, false
	}
	return index, true
}

func (s *Service) validateImageReference(ctx context.Context, userID int64, sessionID int64, taskID int64, imageIndex int) error {
	task, err := s.store.GetJob(ctx, taskID)
	if err != nil {
		return err
	}
	if task.UserID != userID || task.SessionID != sessionID {
		return ErrForbidden
	}
	if task.Status != JobStatusCompleted || task.Result == nil {
		return fmt.Errorf("%w: image task is not completed", ErrInvalidInput)
	}
	if imageIndex < 0 || imageIndex >= len(task.Result.Data) {
		return fmt.Errorf("%w: image index is out of range", ErrInvalidInput)
	}
	if strings.TrimSpace(task.Result.Data[imageIndex].URL) == "" {
		return fmt.Errorf("%w: image url is empty", ErrInvalidInput)
	}
	return nil
}

type userLockPool struct {
	mu    sync.Mutex
	locks map[int64]*sync.Mutex
}

func newUserLockPool() *userLockPool {
	return &userLockPool{locks: map[int64]*sync.Mutex{}}
}

func (p *userLockPool) lock(userID int64) func() {
	p.mu.Lock()
	lock, ok := p.locks[userID]
	if !ok {
		lock = &sync.Mutex{}
		p.locks[userID] = lock
	}
	p.mu.Unlock()

	lock.Lock()
	return lock.Unlock
}

func normalizeQuality(value string) string {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "auto", "low", "medium", "high":
		return trimmed
	default:
		return ""
	}
}

func normalizeSize(value string) string {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "auto":
		return ""
	case "1024x1024", "1024x1536", "1536x1024",
		"1024x1365", "1365x1024", "1088x1920", "1920x1088",
		"2048x2048", "1440x2560", "2560x1440",
		"2160x3840", "3840x2160":
		return trimmed
	default:
		return ""
	}
}

func validateConfig(cfg Config) error {
	if cfg.PlatformConcurrency < 1 {
		return fmt.Errorf("%w: platform_concurrency must be at least 1", ErrInvalidInput)
	}
	if cfg.DefaultUserConcurrency < 1 {
		return fmt.Errorf("%w: default_user_concurrency must be at least 1", ErrInvalidInput)
	}
	if cfg.RetentionDays < 1 {
		return fmt.Errorf("%w: retention_days must be at least 1", ErrInvalidInput)
	}
	if err := validateUnitPrices(cfg.UnitPrices); err != nil {
		return err
	}
	if err := validateUpstreamConfig(cfg.ChatGPT2API); err != nil {
		return err
	}
	return nil
}

func mergeUpstreamConfigInput(current UpstreamConfig, input UpstreamConfigInput) UpstreamConfig {
	baseURL := strings.TrimSpace(input.BaseURL)
	authKey := strings.TrimSpace(current.AuthKey)
	if input.ClearAuthKey {
		authKey = ""
	} else if strings.TrimSpace(input.AuthKey) != "" {
		authKey = strings.TrimSpace(input.AuthKey)
	}
	return UpstreamConfig{
		BaseURL:           baseURL,
		AuthKey:           authKey,
		AuthKeyConfigured: authKey != "",
	}
}

func sanitizeUpstreamConfig(cfg UpstreamConfig) UpstreamConfig {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.AuthKeyConfigured = strings.TrimSpace(cfg.AuthKey) != ""
	cfg.AuthKey = ""
	return cfg
}

func validateUpstreamConfig(cfg UpstreamConfig) error {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: chatgpt2api base_url is invalid", ErrInvalidInput)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return fmt.Errorf("%w: chatgpt2api base_url scheme is invalid", ErrInvalidInput)
	}
}

func validateUserLimit(limit UserLimit) error {
	if limit.UserID <= 0 {
		return fmt.Errorf("%w: user id is required", ErrInvalidInput)
	}
	if limit.Concurrency < 1 {
		return fmt.Errorf("%w: concurrency must be at least 1", ErrInvalidInput)
	}
	return nil
}

func canAccessJob(user runtime.UserProfile, job Job) bool {
	if strings.EqualFold(strings.TrimSpace(user.Role), "admin") {
		return true
	}
	return user.ID > 0 && user.ID == job.UserID
}

func copyInt(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func normalizePriceQuoteInput(input PriceQuoteInput) (PriceQuoteInput, error) {
	model := defaultImageModel
	count := input.N
	if count == 0 {
		count = 1
	}
	resolution := normalizeResolution(input.Resolution)
	if resolution == "" {
		resolution = resolutionFromSize(input.Size)
	}
	if count < 1 || count > maxQueuedImageCount {
		return PriceQuoteInput{}, fmt.Errorf("%w: n must be between 1 and %d", ErrInvalidInput, maxQueuedImageCount)
	}
	return PriceQuoteInput{Model: model, Resolution: resolution, Size: normalizeSize(input.Size), N: count}, nil
}

func normalizeUnitPriceInput(input UnitPriceInput) UnitPrice {
	return UnitPrice{
		OneK:  strings.TrimSpace(input.OneK),
		TwoK:  strings.TrimSpace(input.TwoK),
		FourK: strings.TrimSpace(input.FourK),
	}
}

func withDefaultUnitPrices(price UnitPrice) UnitPrice {
	if strings.TrimSpace(price.OneK) == "" {
		price.OneK = Default1KImageUnitPrice
	}
	if strings.TrimSpace(price.TwoK) == "" {
		price.TwoK = Default2KImageUnitPrice
	}
	if strings.TrimSpace(price.FourK) == "" {
		price.FourK = Default4KImageUnitPrice
	}
	return UnitPrice{
		OneK:  formatDecimalString(price.OneK, 5),
		TwoK:  formatDecimalString(price.TwoK, 5),
		FourK: formatDecimalString(price.FourK, 5),
	}
}

func validateUnitPrices(price UnitPrice) error {
	normalized := withDefaultUnitPrices(price)
	for resolution, value := range map[string]string{
		"one_k":  normalized.OneK,
		"two_k":  normalized.TwoK,
		"four_k": normalized.FourK,
	} {
		if _, err := parseNonNegativeDecimal(value); err != nil {
			return fmt.Errorf("%w: %s unit price is invalid", ErrInvalidInput, resolution)
		}
	}
	return nil
}

func unitPriceForResolution(price UnitPrice, resolution string) string {
	price = withDefaultUnitPrices(price)
	switch resolution {
	case "2k":
		return formatDecimalString(price.TwoK, 5)
	case "4k":
		return formatDecimalString(price.FourK, 5)
	default:
		return formatDecimalString(price.OneK, 5)
	}
}

func normalizeResolution(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1k", "one_k", "one-k":
		return "1k"
	case "2k", "two_k", "two-k":
		return "2k"
	case "4k", "four_k", "four-k":
		return "4k"
	default:
		return ""
	}
}

func resolutionFromSize(size string) string {
	switch normalizeSize(size) {
	case "2048x2048", "1440x2560", "2560x1440":
		return "2k"
	case "2160x3840", "3840x2160":
		return "4k"
	default:
		// auto 和旧任务空 size 都按 1K 档计费，避免缺省配置导致报价为 0。
		return "1k"
	}
}

func multiplyDecimalByInt(value string, multiplier int) (string, error) {
	parsed, err := parseNonNegativeDecimal(value)
	if err != nil {
		return "", err
	}
	return new(big.Rat).Mul(parsed, big.NewRat(int64(multiplier), 1)).FloatString(5), nil
}

func parseNonNegativeDecimal(raw string) (*big.Rat, error) {
	trimmed := strings.TrimSpace(raw)
	if !isPlainDecimal(trimmed) {
		return nil, fmt.Errorf("invalid decimal")
	}
	parsed, ok := new(big.Rat).SetString(trimmed)
	if !ok || parsed.Sign() < 0 {
		return nil, fmt.Errorf("invalid decimal")
	}
	return parsed, nil
}

func formatDecimalString(raw string, precision int) string {
	parsed, err := parseNonNegativeDecimal(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return parsed.FloatString(precision)
}

func isPlainDecimal(value string) bool {
	if value == "" {
		return false
	}
	dotSeen := false
	digits := 0
	fractionDigits := 0
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			digits++
			if dotSeen {
				fractionDigits++
			}
		case r == '.' && !dotSeen:
			dotSeen = true
		default:
			return false
		}
	}
	return digits > 0 && fractionDigits <= 5
}
