package handler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/imagequeue"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/runtime"
	"github.com/gin-gonic/gin"
)

/**
 * TestReadMultipartCreateTaskInputAcceptsTaskImageReference 固化 OpenAI 风格
 * `image` 字段的引用形态，避免 multipart 中的文本字段被误当成缺失文件错误。
 */
func TestReadMultipartCreateTaskInputAcceptsTaskImageReference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	mustWriteField(t, writer, "session_id", "12")
	mustWriteField(t, writer, "model", "gpt-image-2")
	mustWriteField(t, writer, "prompt", "edit the current image")
	mustWriteField(t, writer, "n", "2")
	mustWriteField(t, writer, "size", "1024x1536")
	mustWriteField(t, writer, "quality", "low")
	mustWriteField(t, writer, "output_format", "png")
	mustWriteField(t, writer, "output_compression", "90")
	mustWriteField(t, writer, "publish_to_gallery", "true")
	mustWriteField(t, writer, "image", "task:34:1")
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/custom/images/tasks", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request

	input, err := readMultipartCreateTaskInput(context)
	if err != nil {
		t.Fatalf("readMultipartCreateTaskInput() error = %v", err)
	}
	if input.SessionID != 12 || input.Model != "gpt-image-2" || input.Prompt != "edit the current image" || input.N != 2 {
		t.Fatalf("basic input = %+v", input)
	}
	if input.SourceImageTaskID != 34 || input.SourceImageIndex == nil || *input.SourceImageIndex != 1 {
		t.Fatalf("source image reference = task:%d:%v", input.SourceImageTaskID, input.SourceImageIndex)
	}
	if len(input.SourceImageBytes) != 0 {
		t.Fatalf("source image bytes should be empty for task reference")
	}
	if input.OutputFormat != "png" || input.OutputCompression == nil || *input.OutputCompression != 90 {
		t.Fatalf("output options = %q %v", input.OutputFormat, input.OutputCompression)
	}
	if !input.PublishToGallery {
		t.Fatal("publish_to_gallery should be true")
	}
}

/**
 * TestReadMultipartCreateTaskInputAcceptsImageFile 确认官方 `image` 文件形态仍然可用。
 */
func TestReadMultipartCreateTaskInputAcceptsImageFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	mustWriteField(t, writer, "session_id", "12")
	mustWriteField(t, writer, "prompt", "edit uploaded image")
	mustWriteField(t, writer, "n", "1")
	part, err := writer.CreateFormFile("image", "source.png")
	if err != nil {
		t.Fatalf("create image file field: %v", err)
	}
	if _, err := part.Write([]byte("fake image bytes")); err != nil {
		t.Fatalf("write image file field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/custom/images/tasks", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request

	input, err := readMultipartCreateTaskInput(context)
	if err != nil {
		t.Fatalf("readMultipartCreateTaskInput() error = %v", err)
	}
	if string(input.SourceImageBytes) != "fake image bytes" {
		t.Fatalf("source image bytes = %q", string(input.SourceImageBytes))
	}
	if input.SourceImageFilename != "source.png" {
		t.Fatalf("source image filename = %q", input.SourceImageFilename)
	}
}

// TestTaskSanitizesHistoricalUpstreamErrorMessage 覆盖旧任务失败原因的最后出口兜底。
//
// worker 新失败任务会直接写通用文案，但历史数据可能已经保存上游渠道名；任务详情返回前必须再过滤。
func TestTaskSanitizesHistoricalUpstreamErrorMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &openAIQueueServiceStub{
		taskJob: imagequeue.Job{
			ID:           21,
			UserID:       7,
			Status:       imagequeue.JobStatusFailed,
			ErrorMessage: "chatgpt2api upstream auth key rejected",
		},
	}
	handler := &ImageGenerationHandler{
		userResolver: staticUserResolver{user: runtime.UserProfile{ID: 7}},
		queueService: service,
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/custom/images/tasks/21", nil)
	context.Params = gin.Params{{Key: "id", Value: "21"}}

	handler.Task(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Task imagequeue.Job `json:"task"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Task.ErrorMessage != "生图任务执行失败，请稍后重试" {
		t.Fatalf("error_message = %q", payload.Task.ErrorMessage)
	}
	for _, leaked := range []string{"chatgpt2api", "upstream", "auth key"} {
		if strings.Contains(strings.ToLower(recorder.Body.String()), leaked) {
			t.Fatalf("task response leaked %q: %s", leaked, recorder.Body.String())
		}
	}
}

func mustWriteField(t *testing.T, writer *multipart.Writer, key string, value string) {
	t.Helper()
	if err := writer.WriteField(key, value); err != nil {
		t.Fatalf("write multipart field %s: %v", key, err)
	}
}
