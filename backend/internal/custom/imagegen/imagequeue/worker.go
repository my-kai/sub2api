package imagequeue

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/gallery"
)

const (
	defaultWorkerInterval     = 2 * time.Second
	defaultClaimBatchSize     = 50
	maxUpstreamImageBatchSize = 10
)

// WorkerOptions 保存队列 worker 的可调参数。
type WorkerOptions struct {
	PollInterval   time.Duration
	ClaimBatch     int
	Logger         *log.Logger
	GalleryService GalleryPublisher
}

// GalleryPublisher 是 worker 自动入展厅所需的最小接口。
type GalleryPublisher interface {
	Publish(ctx context.Context, input gallery.UpsertInput) (gallery.Item, error)
}

// Worker 按管理员配置调度 queued 生图任务。
//
// Store 使用 Postgres 原子 UPDATE ... RETURNING claim 任务，可避免多个 worker 重复执行同一任务。
// 如果未来扩成多实例长任务调度，仍建议补充租约或心跳，处理进程存活但 goroutine 卡死的场景。
type Worker struct {
	store       *Store
	service     *Service
	imageClient ImageClient
	interval    time.Duration
	claimBatch  int
	logger      *log.Logger
	gallery     GalleryPublisher

	mu      sync.Mutex
	running map[int64]struct{}
	wg      sync.WaitGroup
}

// NewWorker 创建生图后台调度器。
func NewWorker(store *Store, service *Service, imageClient ImageClient, opts WorkerOptions) *Worker {
	interval := opts.PollInterval
	if interval <= 0 {
		interval = defaultWorkerInterval
	}
	claimBatch := opts.ClaimBatch
	if claimBatch <= 0 {
		claimBatch = defaultClaimBatchSize
	}
	return &Worker{
		store:       store,
		service:     service,
		imageClient: imageClient,
		interval:    interval,
		claimBatch:  claimBatch,
		logger:      opts.Logger,
		gallery:     opts.GalleryService,
		running:     map[int64]struct{}{},
	}
}

// Run 周期性调度队列，直到 context 被取消。
func (w *Worker) Run(ctx context.Context) {
	if w == nil {
		return
	}
	if _, err := w.store.RecoverRunningToQueued(ctx); err != nil {
		w.logf("recover running image generation jobs failed: %v", err)
	}
	if _, err := w.service.CleanupExpiredTerminalJobs(ctx); err != nil {
		w.logf("cleanup image generation jobs failed: %v", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		w.Tick(ctx)
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return
		case <-ticker.C:
		}
	}
}

// Tick 执行一次调度扫描，测试可直接调用它验证并发规则。
func (w *Worker) Tick(ctx context.Context) {
	if w == nil || w.imageClient == nil {
		return
	}

	cfg, err := w.store.GetConfig(ctx)
	if err != nil {
		w.logf("load image generation config failed: %v", err)
		return
	}
	if !cfg.Enabled {
		return
	}
	total, byUser, err := w.store.RunningCounts(ctx)
	if err != nil {
		w.logf("load running image generation counts failed: %v", err)
		return
	}
	queued, err := w.store.ListQueuedJobs(ctx, w.claimBatch)
	if err != nil {
		w.logf("load queued image generation jobs failed: %v", err)
		return
	}

	for _, job := range queued {
		if total >= cfg.PlatformConcurrency {
			return
		}
		userLimit, err := w.store.EffectiveUserConcurrency(ctx, job.UserID, cfg.DefaultUserConcurrency)
		if err != nil {
			w.logf("load image generation user limit failed: %v", err)
			continue
		}
		if byUser[job.UserID] >= userLimit {
			continue
		}

		claimed, ok, err := w.store.ClaimQueuedJob(ctx, job.ID, time.Now().UTC())
		if err != nil {
			w.logf("claim image generation job failed: %v", err)
			continue
		}
		if !ok {
			continue
		}
		total++
		byUser[job.UserID]++
		w.service.PublishTaskEvent()
		w.markLocalRunning(claimed.ID)
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.execute(ctx, claimed)
		}()
	}
}

func (w *Worker) execute(parent context.Context, job Job) {
	defer w.unmarkLocalRunning(job.ID)

	result, err := w.executeJobImages(parent, job)
	if err != nil {
		message := workerErrorMessage(err)
		w.logFailedJob(job, err, message)
		failed, saveErr := w.store.FailJob(contextWithoutCancel(parent), job.ID, message, time.Now().UTC())
		if saveErr != nil {
			w.logf("save failed image generation job failed: job_id=%d user_id=%d reason=%q", job.ID, job.UserID, sanitizeWorkerErrorForLog(saveErr))
		}
		if saveErr == nil {
			if _, refundErr := w.service.RefundFailedJobIfCharged(contextWithoutCancel(parent), failed); refundErr != nil {
				w.logf("refund failed image generation job charge failed: job_id=%d user_id=%d reason=%q", job.ID, job.UserID, sanitizeWorkerErrorForLog(refundErr))
			}
		}
		w.service.PublishTaskEvent()
		return
	}
	if result.Data == nil {
		result.Data = []chatgpt2api.ImageGenerationData{}
	}
	completed, err := w.store.CompleteJob(contextWithoutCancel(parent), job.ID, result, time.Now().UTC())
	if err != nil {
		w.logf("save completed image generation job failed: %v", err)
		return
	}
	if shouldPromoteSessionCurrentImage(completed) {
		if _, err := w.store.SetSessionCurrentImage(contextWithoutCancel(parent), completed.UserID, completed.SessionID, completed.ID, 0, time.Now().UTC()); err != nil {
			w.logf("update image generation session current image failed: %v", err)
		}
	}
	w.publishCompletedJobToGallery(contextWithoutCancel(parent), completed)
	w.service.PublishTaskEvent()
}

func (w *Worker) executeJobImages(ctx context.Context, job Job) (chatgpt2api.ImageGenerationResponse, error) {
	switch job.GenerationMode {
	case "", GenerationModeGenerate:
		return w.generateJobImages(ctx, job)
	case GenerationModeEdit:
		return w.editJobImages(ctx, job)
	default:
		return chatgpt2api.ImageGenerationResponse{}, fmt.Errorf("%w: generation_mode is invalid", chatgpt2api.ErrInvalidRequest)
	}
}

// generateJobImages 按上游单次最多 10 张的限制拆批执行，并把每批结果合并成一个任务结果。
func (w *Worker) generateJobImages(ctx context.Context, job Job) (chatgpt2api.ImageGenerationResponse, error) {
	remaining := job.N
	var merged chatgpt2api.ImageGenerationResponse
	for remaining > 0 {
		batchSize := remaining
		if batchSize > maxUpstreamImageBatchSize {
			batchSize = maxUpstreamImageBatchSize
		}
		request := chatgpt2api.ImageGenerationRequest{
			Model:          job.Model,
			Prompt:         job.Prompt,
			N:              batchSize,
			Quality:        job.Quality,
			Size:           job.Size,
			ResponseFormat: "url",
		}
		result, err := w.imageClient.GenerateImage(ctx, request)
		if err != nil {
			return chatgpt2api.ImageGenerationResponse{}, err
		}
		if merged.Created == 0 {
			merged.Created = result.Created
		}
		merged.Data = append(merged.Data, result.Data...)
		remaining -= batchSize
	}
	if merged.Data == nil {
		merged.Data = []chatgpt2api.ImageGenerationData{}
	}
	return merged, nil
}

// editJobImages 读取任务创建时固定的来源图片，并按上游单次最多 10 张的限制拆批编辑。
func (w *Worker) editJobImages(ctx context.Context, job Job) (chatgpt2api.ImageGenerationResponse, error) {
	sourceImageURL, sourceImageBytes, sourceImageFilename, sourceImageContentType, err := w.editSourceImage(ctx, job)
	if err != nil {
		return chatgpt2api.ImageGenerationResponse{}, err
	}

	remaining := job.N
	var merged chatgpt2api.ImageGenerationResponse
	for remaining > 0 {
		batchSize := remaining
		if batchSize > maxUpstreamImageBatchSize {
			batchSize = maxUpstreamImageBatchSize
		}
		request := chatgpt2api.ImageEditRequest{
			Model:            job.Model,
			Prompt:           job.Prompt,
			N:                batchSize,
			Quality:          job.Quality,
			Size:             job.Size,
			ResponseFormat:   "url",
			ImageURL:         sourceImageURL,
			ImageBytes:       sourceImageBytes,
			ImageFilename:    sourceImageFilename,
			ImageContentType: sourceImageContentType,
		}
		result, err := w.imageClient.EditImage(ctx, request)
		if err != nil {
			return chatgpt2api.ImageGenerationResponse{}, err
		}
		if merged.Created == 0 {
			merged.Created = result.Created
		}
		merged.Data = append(merged.Data, result.Data...)
		remaining -= batchSize
	}
	if merged.Data == nil {
		merged.Data = []chatgpt2api.ImageGenerationData{}
	}
	return merged, nil
}

func (w *Worker) editSourceImage(ctx context.Context, job Job) (string, []byte, string, string, error) {
	if len(job.SourceImageBytes) > 0 {
		filename := strings.TrimSpace(job.SourceImageFilename)
		if filename == "" {
			filename = fmt.Sprintf("image-task-%d-upload.png", job.ID)
		}
		return "", job.SourceImageBytes, filename, job.SourceImageContentType, nil
	}
	sourceImageURL, err := w.sourceImageURL(ctx, job)
	if err != nil {
		return "", nil, "", "", err
	}
	return sourceImageURL, nil, fmt.Sprintf("image-task-%d-source.png", job.ID), "", nil
}

func (w *Worker) sourceImageURL(ctx context.Context, job Job) (string, error) {
	if job.SourceImageTaskID <= 0 || job.SourceImageIndex == nil {
		return "", fmt.Errorf("%w: source image is required", chatgpt2api.ErrInvalidRequest)
	}
	source, err := w.store.GetJob(ctx, job.SourceImageTaskID)
	if err != nil {
		return "", err
	}
	if source.UserID != job.UserID || source.SessionID != job.SessionID {
		return "", fmt.Errorf("%w: source image is not in this session", chatgpt2api.ErrInvalidRequest)
	}
	if source.Status != JobStatusCompleted || source.Result == nil {
		return "", fmt.Errorf("%w: source image task is not completed", chatgpt2api.ErrInvalidRequest)
	}
	index := *job.SourceImageIndex
	if index < 0 || index >= len(source.Result.Data) {
		return "", fmt.Errorf("%w: source image index is out of range", chatgpt2api.ErrInvalidRequest)
	}
	sourceURL := strings.TrimSpace(source.Result.Data[index].URL)
	if sourceURL == "" {
		return "", fmt.Errorf("%w: source image url is empty", chatgpt2api.ErrInvalidRequest)
	}
	return sourceURL, nil
}

func shouldPromoteSessionCurrentImage(job Job) bool {
	return job.SessionID > 0 &&
		job.Status == JobStatusCompleted &&
		job.Result != nil &&
		len(job.Result.Data) > 0 &&
		strings.TrimSpace(job.Result.Data[0].URL) != ""
}

func (w *Worker) publishCompletedJobToGallery(ctx context.Context, job Job) {
	if w.gallery == nil || !job.PublishToGallery || job.Result == nil {
		return
	}
	for index, image := range job.Result.Data {
		imageURL := strings.TrimSpace(image.URL)
		if imageURL == "" {
			continue
		}
		if _, err := w.gallery.Publish(ctx, gallery.UpsertInput{
			UserID:                      job.UserID,
			SourceTaskID:                job.ID,
			SourceImageIndex:            index,
			ImageURL:                    imageURL,
			Prompt:                      job.Prompt,
			CreatedFromPublicGeneration: true,
			PublishedAt:                 time.Now().UTC(),
		}); err != nil {
			w.logf("publish completed image generation job to gallery failed: job_id=%d image_index=%d err=%v", job.ID, index, err)
		}
	}
}

func (w *Worker) localRunningCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.running)
}

func (w *Worker) markLocalRunning(id int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.running[id] = struct{}{}
}

func (w *Worker) unmarkLocalRunning(id int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.running, id)
}

func (w *Worker) logf(format string, args ...any) {
	if w.logger != nil {
		w.logger.Printf(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (w *Worker) logFailedJob(job Job, err error, persistedMessage string) {
	sourceIndex := "-"
	if job.SourceImageIndex != nil {
		sourceIndex = fmt.Sprintf("%d", *job.SourceImageIndex)
	}
	// 不写 prompt 和图片内容，日志只保留定位任务、上游参数和脱敏后的失败原因。
	w.logf(
		"image generation job failed: job_id=%d user_id=%d session_id=%d mode=%q model=%q quality=%q size=%q n=%d source_task_id=%d source_image_index=%s persisted_message=%q reason=%q",
		job.ID,
		job.UserID,
		job.SessionID,
		job.GenerationMode,
		job.Model,
		job.Quality,
		job.Size,
		job.N,
		job.SourceImageTaskID,
		sourceIndex,
		persistedMessage,
		sanitizeWorkerErrorForLog(err),
	)
}

// workerErrorMessage 生成写入任务记录的短错误摘要，避免把完整上游响应或密钥形态落库。
func workerErrorMessage(err error) string {
	switch {
	case errors.Is(err, chatgpt2api.ErrNotConfigured):
		return "chatgpt2api image service is not configured"
	case errors.Is(err, chatgpt2api.ErrInvalidRequest):
		return err.Error()
	case errors.Is(err, chatgpt2api.ErrUnauthorized):
		return "chatgpt2api auth key is invalid or unauthorized"
	default:
		message := strings.TrimSpace(err.Error())
		if message == "" {
			return "image generation failed"
		}
		message = sanitizeWorkerLogText(message)
		if len(message) > 500 {
			return message[:500]
		}
		return message
	}
}

// sanitizeWorkerErrorForLog 保留排障所需错误文本，同时统一移除常见鉴权字段和值。
func sanitizeWorkerErrorForLog(err error) string {
	if err == nil {
		return ""
	}
	return sanitizeWorkerLogText(err.Error())
}

// sanitizeWorkerLogText 同时服务日志和失败摘要，防止 auth-key 只在其中一处被脱敏。
func sanitizeWorkerLogText(raw string) string {
	message := stripWorkerURLSecrets(raw)
	for _, marker := range []string{"auth-key", "auth_key", "x-api-key", "api_key", "apikey", "token", "authorization", "bearer"} {
		message = redactAfterWorkerLogMarker(message, marker)
	}
	return message
}

// redactAfterWorkerLogMarker 隐藏敏感标记后面的值；Authorization/Bearer 一类标记会连同剩余行一起隐藏。
func redactAfterWorkerLogMarker(value string, marker string) string {
	lower := strings.ToLower(value)
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return value
	}

	start := idx + len(marker)
	for start < len(value) && (value[start] == ' ' || value[start] == ':' || value[start] == '=' || value[start] == '"' || value[start] == '\'') {
		start++
	}
	end := start
	if marker == "authorization" || marker == "bearer" {
		for end < len(value) && value[end] != ',' && value[end] != ';' && value[end] != '"' && value[end] != '\'' {
			end++
		}
	} else {
		for end < len(value) && value[end] != ' ' && value[end] != ',' && value[end] != '&' && value[end] != ';' && value[end] != '"' && value[end] != '\'' {
			end++
		}
	}
	if start == end {
		return value
	}
	return value[:start] + "[REDACTED]" + value[end:]
}

// stripWorkerURLSecrets 去掉 URL 查询串和 fragment，避免上游错误把 key 拼进 URL 后写入日志。
func stripWorkerURLSecrets(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return parsed.String()
	}
	if idx := strings.IndexAny(trimmed, "?#"); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}

func contextWithoutCancel(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

// ClaimedRequestForTest 返回任务对应的上游请求，避免测试重复了解 worker 的转换细节。
func ClaimedRequestForTest(job Job) (chatgpt2api.ImageGenerationRequest, error) {
	if job.ID <= 0 {
		return chatgpt2api.ImageGenerationRequest{}, fmt.Errorf("job id is required")
	}
	return chatgpt2api.ImageGenerationRequest{
		Model:          job.Model,
		Prompt:         job.Prompt,
		N:              job.N,
		Quality:        job.Quality,
		Size:           job.Size,
		ResponseFormat: "url",
	}, nil
}
