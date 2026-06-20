package handler

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/imagequeue"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"

	"github.com/gin-gonic/gin"
)

const openAIImageWaitTimeout = 180 * time.Second

// OpenAIImageGenerations 处理 OpenAI 兼容的文生图接口。
//
// 该入口只读取 image key，不依赖主仓登录态；任务创建后同步等待终态，但超时不会取消后台队列任务。
func (h *ImageGenerationHandler) OpenAIImageGenerations(c *gin.Context) {
	user, ok := h.requireOpenAIImageKey(c)
	if !ok {
		return
	}
	var input imagequeue.CreateJobInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeOpenAIImageError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request", "Invalid image generation request.")
		return
	}
	h.createAndWaitOpenAIImageTask(c, user, input)
}

// OpenAIImageEdits 处理 OpenAI 兼容的图片编辑接口。
//
// multipart 文件必须在 HTTP 请求结束前读入队列任务；后台 worker 异步执行时不能再依赖 multipart reader。
func (h *ImageGenerationHandler) OpenAIImageEdits(c *gin.Context) {
	user, ok := h.requireOpenAIImageKey(c)
	if !ok {
		return
	}
	input, err := readOpenAIImageEditInput(c)
	if err != nil {
		writeOpenAIImageError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request", "Invalid image edit request.")
		return
	}
	h.createAndWaitOpenAIImageTask(c, user, input)
}

func (h *ImageGenerationHandler) createAndWaitOpenAIImageTask(c *gin.Context, user runtime.UserProfile, input imagequeue.CreateJobInput) {
	job, err := h.queueService.CreateOpenAITask(c.Request.Context(), user, input)
	if err != nil {
		writeOpenAIQueueError(c, err)
		return
	}
	terminal, err := h.queueService.WaitTaskTerminal(c.Request.Context(), user, job.ID, openAIImageWaitTimeout)
	if err != nil {
		writeOpenAIQueueError(c, err)
		return
	}
	writeOpenAITerminalJob(c, terminal)
}

func readOpenAIImageEditInput(c *gin.Context) (imagequeue.CreateJobInput, error) {
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
	}
	if count, err := strconv.Atoi(strings.TrimSpace(c.PostForm("n"))); err == nil {
		input.N = count
	}
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		return imagequeue.CreateJobInput{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxImageTaskUploadBytes+1))
	if err != nil {
		return imagequeue.CreateJobInput{}, err
	}
	if len(body) > maxImageTaskUploadBytes {
		return imagequeue.CreateJobInput{}, imagequeue.ErrInvalidInput
	}
	input.SourceImageBytes = body
	input.SourceImageFilename = header.Filename
	input.SourceImageContentType = header.Header.Get("Content-Type")
	return input, nil
}

func (h *ImageGenerationHandler) requireOpenAIImageKey(c *gin.Context) (runtime.UserProfile, bool) {
	token, ok := bearerToken(c.GetHeader("Authorization"))
	if !ok {
		writeOpenAIImageError(c, http.StatusUnauthorized, "invalid_request_error", "missing_authorization", "Missing bearer token.")
		return runtime.UserProfile{}, false
	}
	user, _, err := h.queueService.UserForAPIKey(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, imagequeue.ErrAPIKeyNotFound) {
			writeOpenAIImageError(c, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key", "Invalid image API key.")
			return runtime.UserProfile{}, false
		}
		log.Printf("[WARN] custom imagegen openai key lookup failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
		writeOpenAIImageError(c, http.StatusInternalServerError, "api_error", "internal_error", "Image API key lookup failed.")
		return runtime.UserProfile{}, false
	}
	return user, true
}

func bearerToken(header string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(header))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || strings.TrimSpace(fields[1]) == "" {
		return "", false
	}
	return strings.TrimSpace(fields[1]), true
}

func writeOpenAITerminalJob(c *gin.Context, job imagequeue.Job) {
	switch job.Status {
	case imagequeue.JobStatusCompleted:
		if job.Result == nil {
			writeOpenAIImageError(c, http.StatusBadGateway, "api_error", "empty_result", "Image generation completed without result.")
			return
		}
		result := *job.Result
		if result.Created == 0 {
			result.Created = openAIJobCreated(job)
		}
		if result.Data == nil {
			result.Data = []chatgpt2api.ImageGenerationData{}
		}
		c.JSON(http.StatusOK, result)
	case imagequeue.JobStatusCanceled:
		writeOpenAIImageError(c, http.StatusConflict, "request_error", "task_canceled", "Image generation task was canceled.")
	default:
		writeOpenAIImageError(c, http.StatusBadGateway, "api_error", "image_generation_failed", publicOpenAITerminalJobMessage(job))
	}
}

// publicOpenAITerminalJobMessage 把内部任务失败原因映射为 OpenAI 兼容接口的稳定业务文案。
//
// 任务 `error_message` 可能来自旧数据或历史版本 worker，不能信任其已脱敏；兼容接口作为外部调用面，
// 这里再兜底过滤一次，避免把渠道名称、鉴权状态或上游原始错误返回给调用方。
func publicOpenAITerminalJobMessage(job imagequeue.Job) string {
	if job.Status != imagequeue.JobStatusFailed {
		return "Image generation task failed."
	}
	return "Image generation task failed. Please try again later."
}

func writeOpenAIQueueError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, imagequeue.ErrInvalidInput):
		writeOpenAIImageError(c, http.StatusBadRequest, "invalid_request_error", "invalid_request", err.Error())
	case errors.Is(err, imagequeue.ErrInsufficientBalance), errors.Is(err, imagequeue.ErrBalanceChargeFailed):
		writeOpenAIImageError(c, http.StatusPaymentRequired, "billing_error", "insufficient_balance", "Insufficient balance.")
	case errors.Is(err, imagequeue.ErrDisabled):
		writeOpenAIImageError(c, http.StatusServiceUnavailable, "api_error", "service_disabled", "Image generation is disabled.")
	case errors.Is(err, imagequeue.ErrSessionNotFound), errors.Is(err, imagequeue.ErrJobNotFound):
		writeOpenAIImageError(c, http.StatusNotFound, "invalid_request_error", "not_found", "Image generation task not found.")
	case errors.Is(err, imagequeue.ErrForbidden):
		writeOpenAIImageError(c, http.StatusForbidden, "invalid_request_error", "forbidden", "Access to image generation task is forbidden.")
	case errors.Is(err, imagequeue.ErrTaskWaitTimeout):
		writeOpenAIImageError(c, http.StatusGatewayTimeout, "api_error", "timeout", "Image generation task timed out.")
	default:
		log.Printf("[WARN] custom imagegen openai request failed: reason=%q path=%q client_ip=%q", sanitizeErrorForLog(err), c.Request.URL.Path, c.ClientIP())
		writeOpenAIImageError(c, http.StatusInternalServerError, "api_error", "internal_error", "Image generation service error.")
	}
}

func writeOpenAIImageError(c *gin.Context, status int, errorType string, code string, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errorType,
			"param":   nil,
			"code":    code,
		},
	})
}

func openAIJobCreated(job imagequeue.Job) int64 {
	if job.FinishedAt != nil && !job.FinishedAt.IsZero() {
		return job.FinishedAt.Unix()
	}
	if !job.CreatedAt.IsZero() {
		return job.CreatedAt.Unix()
	}
	return time.Now().Unix()
}
