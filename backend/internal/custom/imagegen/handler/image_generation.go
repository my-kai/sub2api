package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/gallery"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/imagequeue"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"

	"github.com/gin-gonic/gin"
)

// ImageQueueService 是 handler 需要的生图队列业务窄接口。
type ImageQueueService interface {
	CreateSession(ctx context.Context, user runtime.UserProfile, input imagequeue.CreateSessionInput) (imagequeue.Session, error)
	Sessions(ctx context.Context, user runtime.UserProfile) ([]imagequeue.Session, error)
	UpdateSession(ctx context.Context, user runtime.UserProfile, sessionID int64, input imagequeue.UpdateSessionInput) (imagequeue.Session, error)
	DeleteSession(ctx context.Context, user runtime.UserProfile, sessionID int64) error
	SetCurrentImage(ctx context.Context, user runtime.UserProfile, sessionID int64, input imagequeue.SetCurrentImageInput) (imagequeue.Session, error)
	ResetCurrentImage(ctx context.Context, user runtime.UserProfile, sessionID int64) (imagequeue.Session, error)
	SessionTasks(ctx context.Context, user runtime.UserProfile, sessionID int64, page imagequeue.PageRequest) (imagequeue.PageResult[imagequeue.Job], error)
	SubscribeTaskEvents(ctx context.Context) (<-chan imagequeue.TaskEvent, func())
	MyImages(ctx context.Context, user runtime.UserProfile, page imagequeue.PageRequest) (imagequeue.PageResult[imagequeue.MyImage], error)
	CreateTask(ctx context.Context, user runtime.UserProfile, input imagequeue.CreateJobInput) (imagequeue.Job, error)
	RetryTask(ctx context.Context, user runtime.UserProfile, id int64) (imagequeue.Job, error)
	Task(ctx context.Context, user runtime.UserProfile, id int64) (imagequeue.Job, error)
	CancelTask(ctx context.Context, user runtime.UserProfile, id int64) (imagequeue.Job, error)
	Config(ctx context.Context) (imagequeue.Config, error)
	PublicStatus(ctx context.Context) (imagequeue.PublicStatus, error)
	QuotePrice(ctx context.Context, input imagequeue.PriceQuoteInput) (imagequeue.PriceQuote, error)
	APIKeys(ctx context.Context, user runtime.UserProfile) ([]imagequeue.APIKey, error)
	CreateAPIKey(ctx context.Context, user runtime.UserProfile, input imagequeue.CreateAPIKeyInput) (imagequeue.APIKey, error)
	DeleteAPIKey(ctx context.Context, user runtime.UserProfile, id int64) error
	UserForAPIKey(ctx context.Context, plaintext string) (runtime.UserProfile, imagequeue.APIKey, error)
	CreateOpenAITask(ctx context.Context, user runtime.UserProfile, input imagequeue.CreateJobInput) (imagequeue.Job, error)
	WaitTaskTerminal(ctx context.Context, user runtime.UserProfile, id int64, timeout time.Duration) (imagequeue.Job, error)
	UpdateConfig(ctx context.Context, input imagequeue.ConfigInput, admin runtime.UserProfile) (imagequeue.Config, error)
	UserLimits(ctx context.Context) ([]imagequeue.UserLimit, error)
	UpsertUserLimit(ctx context.Context, userID int64, input imagequeue.UserLimitInput, snapshot imagequeue.UserLimitSnapshot) (imagequeue.UserLimit, error)
	DeleteUserLimit(ctx context.Context, userID int64) error
}

// PublicGalleryService 是公开展厅读写所需的窄接口。
type PublicGalleryService interface {
	VisibleList(ctx context.Context, page int, pageSize int, includePrompt bool) ([]gallery.ListItem, int64, error)
	Publish(ctx context.Context, input gallery.UpsertInput) (gallery.Item, error)
	Hide(ctx context.Context, sourceTaskID int64, sourceImageIndex int) (gallery.Item, error)
	ItemBySource(ctx context.Context, sourceTaskID int64, sourceImageIndex int) (gallery.Item, error)
}

// AdminUserLookupClient 是用户并发覆盖选择器读取上游用户的窄接口。
type AdminUserLookupClient interface {
	SearchAdminUsers(ctx context.Context, query string, limit int) ([]runtime.AdminUserSummary, error)
	GetAdminUser(ctx context.Context, userID int64) (runtime.AdminUserSummary, error)
}

const maxImageTaskUploadBytes = 64 << 20

// ImageGenerationHandler 暴露同源生图队列接口。
type ImageGenerationHandler struct {
	userResolver     runtime.UserResolver
	userLookupClient AdminUserLookupClient
	queueService     ImageQueueService
	galleryService   PublicGalleryService
}

const (
	imageTaskEventPollInterval = 15 * time.Second
	imageTaskEventPingInterval = 30 * time.Second
)

// NewImageGenerationHandler 装配生图处理器依赖。
func NewImageGenerationHandler(userResolver runtime.UserResolver, queueService ImageQueueService) *ImageGenerationHandler {
	return &ImageGenerationHandler{userResolver: userResolver, queueService: queueService}
}

// WithGalleryService 安装公开展厅依赖。
func (h *ImageGenerationHandler) WithGalleryService(service PublicGalleryService) *ImageGenerationHandler {
	if h != nil {
		h.galleryService = service
	}
	return h
}

// WithAdminUserLookup 安装管理员用户候选查询依赖。
func (h *ImageGenerationHandler) WithAdminUserLookup(client AdminUserLookupClient) *ImageGenerationHandler {
	if h != nil {
		h.userLookupClient = client
	}
	return h
}

// PublicStatus 返回普通用户可读取的生图功能开关状态。
func (h *ImageGenerationHandler) PublicStatus(c *gin.Context) {
	if _, ok := h.requireUser(c); !ok {
		return
	}
	status, err := h.queueService.PublicStatus(c.Request.Context())
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

// Models 返回 custom 生图当前开放的固定模型。
func (h *ImageGenerationHandler) Models(c *gin.Context) {
	if _, ok := h.requireUser(c); !ok {
		return
	}

	c.JSON(http.StatusOK, chatgpt2api.ModelsResponse{Data: []chatgpt2api.Model{{
		ID:      imagequeue.DefaultImageModel,
		Object:  "model",
		OwnedBy: "custom",
	}}})
}

// CreateSession 创建一个服务端持久化 Session，供后续任务绑定上下文。
func (h *ImageGenerationHandler) CreateSession(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	var input imagequeue.CreateSessionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid image session request"})
		return
	}
	session, err := h.queueService.CreateSession(c.Request.Context(), user, input)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"session": session})
}

// Sessions 返回当前用户自己的 Session 列表，空数据固定为 []。
func (h *ImageGenerationHandler) Sessions(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	items, err := h.queueService.Sessions(c.Request.Context(), user)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	if items == nil {
		// 后端 AGENTS 要求数组字段永远返回 []，避免前端列表读取 null。
		items = []imagequeue.Session{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// UpdateSession 修改当前用户自己的 Session 标题。
func (h *ImageGenerationHandler) UpdateSession(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	sessionID, ok := parseImageSessionID(c)
	if !ok {
		return
	}
	var input imagequeue.UpdateSessionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid image session request"})
		return
	}
	session, err := h.queueService.UpdateSession(c.Request.Context(), user, sessionID, input)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": session})
}

// DeleteSession 软删除当前用户自己的 Session，任务和图片结果仍保留。
func (h *ImageGenerationHandler) DeleteSession(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	sessionID, ok := parseImageSessionID(c)
	if !ok {
		return
	}
	if err := h.queueService.DeleteSession(c.Request.Context(), user, sessionID); err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// SetCurrentImage 将已完成任务中的某张图片设为当前 Session 后续编辑的来源图片。
func (h *ImageGenerationHandler) SetCurrentImage(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	sessionID, ok := parseImageSessionID(c)
	if !ok {
		return
	}
	var input imagequeue.SetCurrentImageInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid image reference request"})
		return
	}
	session, err := h.queueService.SetCurrentImage(c.Request.Context(), user, sessionID, input)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": session})
}

// ResetCurrentImage 取消手动指定图片，并恢复到当前会话默认最新编辑图。
func (h *ImageGenerationHandler) ResetCurrentImage(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	sessionID, ok := parseImageSessionID(c)
	if !ok {
		return
	}
	session, err := h.queueService.ResetCurrentImage(c.Request.Context(), user, sessionID)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": session})
}

// SessionTasks 返回当前 Session 的任务流分页。
func (h *ImageGenerationHandler) SessionTasks(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	sessionID, ok := parseImageSessionID(c)
	if !ok {
		return
	}
	page := parseImagePageRequest(c)
	result, err := h.queueService.SessionTasks(c.Request.Context(), user, sessionID, page)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	if result.Items == nil {
		result.Items = []imagequeue.Job{}
	}
	sanitizePublicImageJobs(result.Items)
	c.JSON(http.StatusOK, result)
}

// SessionTaskEvents 通过 SSE 推送当前 Session 的任务快照。
//
// 事件总线负责快速唤醒，低频兜底负责覆盖进程重启、浏览器重连或未来多实例部署下漏掉的内存事件。
func (h *ImageGenerationHandler) SessionTaskEvents(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	sessionID, ok := parseImageSessionID(c)
	if !ok {
		return
	}
	page := parseImagePageRequest(c)

	if _, err := h.queueService.SessionTasks(c.Request.Context(), user, sessionID, page); err != nil {
		writeImageQueueError(c, err)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	events, cleanup := h.queueService.SubscribeTaskEvents(c.Request.Context())
	defer cleanup()

	pollTicker := time.NewTicker(imageTaskEventPollInterval)
	defer pollTicker.Stop()
	pingTicker := time.NewTicker(imageTaskEventPingInterval)
	defer pingTicker.Stop()

	lastSnapshot := ""
	writeSnapshot := func() bool {
		result, err := h.queueService.SessionTasks(c.Request.Context(), user, sessionID, page)
		if err != nil {
			h.writeSSE(c, "snapshot_error", gin.H{"message": imageQueueSSEErrorMessage(err)})
			return false
		}
		if result.Items == nil {
			result.Items = []imagequeue.Job{}
		}
		sanitizePublicImageJobs(result.Items)
		encoded, err := json.Marshal(result)
		if err != nil {
			h.writeSSE(c, "snapshot_error", gin.H{"message": "任务读取失败"})
			return false
		}
		snapshot := string(encoded)
		if snapshot == lastSnapshot {
			return true
		}
		lastSnapshot = snapshot
		h.writeSSE(c, "tasks", gin.H{
			"tasks":   result,
			"balance": currentBalance(),
		})
		return true
	}

	if !writeSnapshot() {
		return
	}
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case _, open := <-events:
			if !open {
				return
			}
			if !writeSnapshot() {
				return
			}
		case <-pollTicker.C:
			if !writeSnapshot() {
				return
			}
		case <-pingTicker.C:
			h.writeSSE(c, "ping", gin.H{})
		}
	}
}

// MyImages 返回当前用户已完成任务里的图片分页。
func (h *ImageGenerationHandler) MyImages(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	result, err := h.queueService.MyImages(c.Request.Context(), user, parseImagePageRequest(c))
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	if result.Items == nil {
		result.Items = []imagequeue.MyImage{}
	}
	c.JSON(http.StatusOK, result)
}

// CreateTask 创建一个持久化生图任务，后续由 worker 按并发配置执行。
func (h *ImageGenerationHandler) CreateTask(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}

	input, err := readCreateTaskInput(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid image generation request"})
		return
	}

	job, err := h.queueService.CreateTask(c.Request.Context(), user, input)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, h.taskResponse(job))
}

func readCreateTaskInput(c *gin.Context) (imagequeue.CreateJobInput, error) {
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return readMultipartCreateTaskInput(c)
	}
	var input imagequeue.CreateJobInput
	if err := c.ShouldBindJSON(&input); err != nil {
		return imagequeue.CreateJobInput{}, err
	}
	return input, nil
}

func readMultipartCreateTaskInput(c *gin.Context) (imagequeue.CreateJobInput, error) {
	if err := c.Request.ParseMultipartForm(maxImageTaskUploadBytes); err != nil {
		return imagequeue.CreateJobInput{}, err
	}
	input := imagequeue.CreateJobInput{
		Model:             c.PostForm("model"),
		Prompt:            c.PostForm("prompt"),
		Quality:           c.PostForm("quality"),
		Size:              c.PostForm("size"),
		OutputFormat:      c.PostForm("output_format"),
		OutputCompression: parseOptionalIntFormValue(c.PostForm("output_compression")),
		PublishToGallery:  parseBoolFormValue(c.PostForm("publish_to_gallery")),
	}
	if sessionID, err := strconv.ParseInt(strings.TrimSpace(c.PostForm("session_id")), 10, 64); err == nil {
		input.SessionID = sessionID
	}
	if count, err := strconv.Atoi(strings.TrimSpace(c.PostForm("n"))); err == nil {
		input.N = count
	}
	if taskID, imageIndex, ok := parseTaskImageFormReference(c.PostForm("image")); ok {
		input.SourceImageTaskID = taskID
		input.SourceImageIndex = &imageIndex
	}

	file, header, err := c.Request.FormFile("image")
	if err != nil && errors.Is(err, http.ErrMissingFile) {
		return input, nil
	}
	if err != nil {
		return imagequeue.CreateJobInput{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxImageTaskUploadBytes+1))
	if err != nil {
		return imagequeue.CreateJobInput{}, err
	}
	if len(body) > maxImageTaskUploadBytes {
		return imagequeue.CreateJobInput{}, fmt.Errorf("image is too large")
	}
	input.SourceImageBytes = body
	input.SourceImageFilename = header.Filename
	input.SourceImageContentType = header.Header.Get("Content-Type")
	return input, nil
}

func parseOptionalIntFormValue(value string) *int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseTaskImageFormReference(value string) (int64, int, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "task:") {
		return 0, 0, false
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) != 3 {
		return 0, 0, false
	}
	taskID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || taskID <= 0 {
		return 0, 0, false
	}
	imageIndex, err := strconv.Atoi(parts[2])
	if err != nil || imageIndex < 0 {
		return 0, 0, false
	}
	return taskID, imageIndex, true
}

func parseBoolFormValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// PublishMyImage 把当前用户的一张图片加入公开展厅。
func (h *ImageGenerationHandler) PublishMyImage(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	if h.galleryService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "public gallery service is unavailable"})
		return
	}
	taskID, imageIndex, ok := parseMyImageRef(c)
	if !ok {
		return
	}
	job, err := h.queueService.Task(c.Request.Context(), user, taskID)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	if job.UserID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"message": "无权管理该图片"})
		return
	}
	imageURL, err := taskImageURL(job, imageIndex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	item, err := h.galleryService.Publish(c.Request.Context(), gallery.UpsertInput{
		UserID:                      user.ID,
		SourceTaskID:                job.ID,
		SourceImageIndex:            imageIndex,
		ImageURL:                    imageURL,
		Prompt:                      job.Prompt,
		CreatedFromPublicGeneration: job.PublishToGallery,
		PublishedAt:                 time.Now().UTC(),
	})
	if err != nil {
		writeGalleryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"gallery_item": gin.H{"id": item.ID, "in_gallery": item.IsVisible}})
}

// HideMyImage 将当前用户的一张图片从展厅隐藏。
func (h *ImageGenerationHandler) HideMyImage(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	if h.galleryService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "public gallery service is unavailable"})
		return
	}
	taskID, imageIndex, ok := parseMyImageRef(c)
	if !ok {
		return
	}
	job, err := h.queueService.Task(c.Request.Context(), user, taskID)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	if job.UserID != user.ID {
		c.JSON(http.StatusForbidden, gin.H{"message": "无权管理该图片"})
		return
	}
	item, err := h.galleryService.Hide(c.Request.Context(), taskID, imageIndex)
	if err != nil {
		writeGalleryError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"gallery_item": gin.H{"id": item.ID, "in_gallery": item.IsVisible}})
}

// PublicGallery 返回公开展厅分页列表；匿名用户也可访问，但提示词只对已登录用户可见。
func (h *ImageGenerationHandler) PublicGallery(c *gin.Context) {
	if h.galleryService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "public gallery service is unavailable"})
		return
	}
	page := parseImagePageRequest(c)
	user, _ := h.optionalUser(c)
	includePrompt := user.ID > 0
	items, total, err := h.galleryService.VisibleList(c.Request.Context(), page.Page, page.PageSize, includePrompt)
	if err != nil {
		writeGalleryError(c, err)
		return
	}
	if items == nil {
		items = []gallery.ListItem{}
	}
	c.JSON(http.StatusOK, gin.H{
		"page":      page.Page,
		"page_size": page.PageSize,
		"total":     total,
		"pages":     pageCount(total, page.PageSize),
		"items":     items,
	})
}

// RetryTask 复用失败任务参数创建新的排队任务。
func (h *ImageGenerationHandler) RetryTask(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, ok := parseImageTaskID(c)
	if !ok {
		return
	}

	job, err := h.queueService.RetryTask(c.Request.Context(), user, id)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, h.taskResponse(job))
}

// Task 返回当前任务状态和排队位置。
func (h *ImageGenerationHandler) Task(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, ok := parseImageTaskID(c)
	if !ok {
		return
	}

	job, err := h.queueService.Task(c.Request.Context(), user, id)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.taskResponse(job))
}

// CancelTask 只允许撤销尚未进入执行阶段的 queued 任务。
func (h *ImageGenerationHandler) CancelTask(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, ok := parseImageTaskID(c)
	if !ok {
		return
	}

	job, err := h.queueService.CancelTask(c.Request.Context(), user, id)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.taskResponse(job))
}

// PriceQuote 返回当前生图参数对应的图片额度预览。
func (h *ImageGenerationHandler) PriceQuote(c *gin.Context) {
	if _, ok := h.requireUser(c); !ok {
		return
	}
	var input imagequeue.PriceQuoteInput
	if err := c.ShouldBindQuery(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid image price quote request"})
		return
	}
	quote, err := h.queueService.QuotePrice(c.Request.Context(), input)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, quote)
}

// APIKeys 返回当前用户自己的生图 Key 脱敏列表。
func (h *ImageGenerationHandler) APIKeys(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	items, err := h.queueService.APIKeys(c.Request.Context(), user)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	if items == nil {
		items = []imagequeue.APIKey{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// CreateAPIKey 创建生图 Key；完整明文只在本次响应中返回。
func (h *ImageGenerationHandler) CreateAPIKey(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	var input imagequeue.CreateAPIKeyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid image api key request"})
		return
	}
	key, err := h.queueService.CreateAPIKey(c.Request.Context(), user, input)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"api_key": key})
}

// DeleteAPIKey 软删除当前用户自己的生图 Key。
func (h *ImageGenerationHandler) DeleteAPIKey(c *gin.Context) {
	user, ok := h.requireUser(c)
	if !ok {
		return
	}
	id, ok := parseAPIKeyID(c)
	if !ok {
		return
	}
	if err := h.queueService.DeleteAPIKey(c.Request.Context(), user, id); err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// AdminConfig 返回管理员可见的生图并发配置。
func (h *ImageGenerationHandler) AdminConfig(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	cfg, err := h.queueService.Config(c.Request.Context())
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateAdminConfig 校验并保存生图并发配置。
func (h *ImageGenerationHandler) UpdateAdminConfig(c *gin.Context) {
	admin, ok := h.requireAdmin(c)
	if !ok {
		return
	}
	var input imagequeue.ConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid image queue config request"})
		return
	}
	cfg, err := h.queueService.UpdateConfig(c.Request.Context(), input, admin)
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UserLimits 返回所有用户并发覆盖。
func (h *ImageGenerationHandler) UserLimits(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	items, err := h.queueService.UserLimits(c.Request.Context())
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	if items == nil {
		// 后端 AGENTS 要求数组字段永远返回 []，避免前端表格读取 null。
		items = []imagequeue.UserLimit{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// SearchAdminUsers 返回管理员选择并发覆盖用户时使用的候选项。
func (h *ImageGenerationHandler) SearchAdminUsers(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	if h.userLookupClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "用户搜索暂时不可用"})
		return
	}

	query := strings.TrimSpace(c.Query("q"))
	limit := parseUserSearchLimit(c.Query("limit"))
	items, err := h.searchAdminUserOptions(c.Request.Context(), query, limit)
	if err != nil {
		writeAdminUserLookupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// UpsertUserLimit 保存指定用户的并发覆盖。
func (h *ImageGenerationHandler) UpsertUserLimit(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	var input imagequeue.UserLimitInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid image queue user limit request"})
		return
	}
	limit, err := h.queueService.UpsertUserLimit(c.Request.Context(), userID, input, h.userLimitSnapshot(c.Request.Context(), userID))
	if err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, limit)
}

// DeleteUserLimit 删除指定用户的并发覆盖。
func (h *ImageGenerationHandler) DeleteUserLimit(c *gin.Context) {
	if _, ok := h.requireAdmin(c); !ok {
		return
	}
	userID, ok := parseUserID(c)
	if !ok {
		return
	}
	if err := h.queueService.DeleteUserLimit(c.Request.Context(), userID); err != nil {
		writeImageQueueError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *ImageGenerationHandler) searchAdminUserOptions(ctx context.Context, query string, limit int) ([]runtime.AdminUserSummary, error) {
	items := make([]runtime.AdminUserSummary, 0, limit)
	seen := make(map[int64]bool)

	// 数字搜索优先按用户 ID 精确读取，补齐上游列表接口只按邮箱/用户名模糊匹配的情况。
	if id, ok := parsePositiveInt64(query); ok {
		user, err := h.userLookupClient.GetAdminUser(ctx, id)
		if err == nil && user.ID > 0 {
			items = append(items, user)
			seen[user.ID] = true
		} else if errors.Is(err, runtime.ErrUnauthorized) {
			return nil, err
		}
	}

	list, err := h.userLookupClient.SearchAdminUsers(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	for _, item := range list {
		if item.ID <= 0 || seen[item.ID] {
			continue
		}
		items = append(items, item)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (h *ImageGenerationHandler) userLimitSnapshot(ctx context.Context, userID int64) imagequeue.UserLimitSnapshot {
	if h.userLookupClient == nil {
		return imagequeue.UserLimitSnapshot{}
	}
	user, err := h.userLookupClient.GetAdminUser(ctx, userID)
	if err != nil {
		// 保存并发值是主动作；用户资料只是展示快照，读取失败不能阻断管理员修正并发。
		log.Printf("[WARN] custom imagegen admin user lookup failed: user_id=%d reason=%q", userID, sanitizeErrorForLog(err))
		return imagequeue.UserLimitSnapshot{}
	}
	return imagequeue.UserLimitSnapshot{Username: user.Username, Email: user.Email}
}

func (h *ImageGenerationHandler) taskResponse(job imagequeue.Job) gin.H {
	sanitizePublicImageJob(&job)
	return gin.H{
		"task":    job,
		"balance": currentBalance(),
	}
}

func (h *ImageGenerationHandler) writeSSE(c *gin.Context, event string, payload any) {
	c.SSEvent(event, payload)
	c.Writer.Flush()
}

func imageQueueSSEErrorMessage(err error) string {
	switch {
	case errors.Is(err, imagequeue.ErrSessionNotFound):
		return "生图 Session 不存在"
	case errors.Is(err, imagequeue.ErrForbidden):
		return "无权访问该生图任务"
	default:
		return "任务读取失败"
	}
}

func currentBalance() float64 {
	// custom 生图不接主仓余额核心；balance 仅作为旧前端兼容字段保留。
	return 0
}

func (h *ImageGenerationHandler) requireUser(c *gin.Context) (runtime.UserProfile, bool) {
	if h.userResolver == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "image generation service is unavailable"})
		return runtime.UserProfile{}, false
	}
	user, err := h.userResolver.RequireUser(c)
	if err != nil {
		if errors.Is(err, runtime.ErrUnauthorized) {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "登录失效，请重新登录"})
			return runtime.UserProfile{}, false
		}
		if errors.Is(err, runtime.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"message": "权限不足"})
			return runtime.UserProfile{}, false
		}
		log.Printf("[WARN] custom imagegen user resolver failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
		c.JSON(http.StatusInternalServerError, gin.H{"message": "用户信息读取失败"})
		return runtime.UserProfile{}, false
	}
	return user, true
}

func (h *ImageGenerationHandler) requireAdmin(c *gin.Context) (runtime.UserProfile, bool) {
	if h.userResolver == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "image generation service is unavailable"})
		return runtime.UserProfile{}, false
	}
	user, err := h.userResolver.RequireAdmin(c)
	if err != nil {
		if errors.Is(err, runtime.ErrUnauthorized) {
			c.JSON(http.StatusUnauthorized, gin.H{"message": "登录失效，请重新登录"})
			return runtime.UserProfile{}, false
		}
		if errors.Is(err, runtime.ErrForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"message": "admin role is required"})
			return runtime.UserProfile{}, false
		}
		log.Printf("[WARN] custom imagegen admin resolver failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
		c.JSON(http.StatusInternalServerError, gin.H{"message": "用户信息读取失败"})
		return runtime.UserProfile{}, false
	}
	return user, true
}

func (h *ImageGenerationHandler) optionalUser(c *gin.Context) (runtime.UserProfile, bool) {
	if h.userResolver == nil {
		return runtime.UserProfile{}, false
	}
	return h.userResolver.OptionalUser(c)
}

func writeImageGenerationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, chatgpt2api.ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case isChatGPT2APIUpstreamError(err) || errors.Is(err, chatgpt2api.ErrNotConfigured) || errors.Is(err, chatgpt2api.ErrUnauthorized):
		log.Printf("[WARN] chatgpt2api image generation failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
		c.JSON(http.StatusBadGateway, gin.H{"message": publicImageTaskFailureMessage})
	default:
		log.Printf("[WARN] image generation service failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
		c.JSON(http.StatusInternalServerError, gin.H{"message": "生图服务暂时不可用"})
	}
}

func writeImageQueueError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, imagequeue.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
	case errors.Is(err, imagequeue.ErrBalanceNotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "余额服务暂时不可用"})
	case errors.Is(err, imagequeue.ErrInsufficientBalance):
		c.JSON(http.StatusPaymentRequired, gin.H{"message": "余额不足，无法创建生图任务"})
	case errors.Is(err, imagequeue.ErrBalanceChargeFailed):
		c.JSON(http.StatusPaymentRequired, gin.H{"message": "余额不足，无法创建生图任务"})
	case errors.Is(err, imagequeue.ErrBalanceRefundFailed):
		c.JSON(http.StatusBadGateway, gin.H{"message": "任务已更新，余额稍后处理"})
	case errors.Is(err, imagequeue.ErrDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "生图功能已关闭"})
	case errors.Is(err, imagequeue.ErrSessionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "生图 Session 不存在"})
	case errors.Is(err, imagequeue.ErrJobNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "生图任务不存在"})
	case errors.Is(err, imagequeue.ErrAPIKeyNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "生图 Key 不存在"})
	case errors.Is(err, imagequeue.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"message": "无权访问该生图任务"})
	case errors.Is(err, imagequeue.ErrCancelNotAllowed):
		c.JSON(http.StatusConflict, gin.H{"message": "任务已开始生成，不能撤销"})
	case errors.Is(err, imagequeue.ErrRetryNotAllowed):
		c.JSON(http.StatusConflict, gin.H{"message": "只有失败任务可以重试"})
	default:
		log.Printf("[WARN] image queue service failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
		c.JSON(http.StatusInternalServerError, gin.H{"message": "image queue service error"})
	}
}

func parseImageSessionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "生图 Session 编号无效"})
		return 0, false
	}
	return id, true
}

func parseImageTaskID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "生图任务编号无效"})
		return 0, false
	}
	return id, true
}

func parseMyImageRef(c *gin.Context) (int64, int, bool) {
	taskID, err := strconv.ParseInt(c.Param("task_id"), 10, 64)
	if err != nil || taskID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "生图任务编号无效"})
		return 0, 0, false
	}
	imageIndex, err := strconv.Atoi(c.Param("image_index"))
	if err != nil || imageIndex < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "图片编号无效"})
		return 0, 0, false
	}
	return taskID, imageIndex, true
}

func parseAPIKeyID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "生图 Key 编号无效"})
		return 0, false
	}
	return id, true
}

func taskImageURL(job imagequeue.Job, imageIndex int) (string, error) {
	if job.Status != imagequeue.JobStatusCompleted || job.Result == nil {
		return "", fmt.Errorf("图片尚未生成完成")
	}
	if imageIndex < 0 || imageIndex >= len(job.Result.Data) {
		return "", fmt.Errorf("图片编号超出范围")
	}
	imageURL := strings.TrimSpace(job.Result.Data[imageIndex].URL)
	if imageURL == "" {
		return "", fmt.Errorf("图片地址为空")
	}
	return imageURL, nil
}

func pageCount(total int64, pageSize int) int {
	if pageSize <= 0 || total <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

func writeGalleryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gallery.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"message": "展厅图片不存在"})
	case errors.Is(err, gallery.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"message": "无权管理该图片"})
	case errors.Is(err, gallery.ErrBadInput):
		c.JSON(http.StatusBadRequest, gin.H{"message": "展厅图片参数无效"})
	default:
		log.Printf("[WARN] public gallery failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
		c.JSON(http.StatusInternalServerError, gin.H{"message": "public gallery service error"})
	}
}

func parseImagePageRequest(c *gin.Context) imagequeue.PageRequest {
	page := parsePositiveQueryInt(c.Query("page"))
	if page <= 0 {
		page = 1
	}
	pageSize := parsePositiveQueryInt(c.Query("page_size"))
	if pageSize <= 0 || pageSize > 20 {
		pageSize = 20
	}
	return imagequeue.PageRequest{
		Page:     page,
		PageSize: pageSize,
	}
}

func parseUserID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid user id"})
		return 0, false
	}
	return id, true
}

func parseUserSearchLimit(raw string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func parsePositiveInt64(raw string) (int64, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value, err == nil && value > 0
}

func parsePositiveQueryInt(raw string) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func writeAdminUserLookupError(c *gin.Context, err error) {
	if errors.Is(err, runtime.ErrUnauthorized) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "登录失效，请重新登录"})
		return
	}
	log.Printf("[WARN] custom imagegen admin user search failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
	c.JSON(http.StatusBadGateway, gin.H{"message": "用户搜索失败"})
}

func isChatGPT2APIUpstreamError(err error) bool {
	var upstreamErr *chatgpt2api.UpstreamError
	return errors.As(err, &upstreamErr)
}
