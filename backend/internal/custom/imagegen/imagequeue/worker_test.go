package imagequeue

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
)

type retryImageClient struct {
	generate func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error)
	edit     func(context.Context, chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error)
}

func (c retryImageClient) GenerateImage(ctx context.Context, input chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
	if c.generate == nil {
		return chatgpt2api.ImageGenerationResponse{}, errors.New("generate image mock is not configured")
	}
	return c.generate(ctx, input)
}

func (c retryImageClient) EditImage(ctx context.Context, input chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error) {
	if c.edit == nil {
		return chatgpt2api.ImageGenerationResponse{}, errors.New("edit image mock is not configured")
	}
	return c.edit(ctx, input)
}

// TestWorkerGenerateRetriesNoAvailableImageQuota 固化上游临时无额度时的文生图重试行为。
func TestWorkerGenerateRetriesNoAvailableImageQuota(t *testing.T) {
	restoreImageQuotaRetryDelay(t)
	calls := 0
	worker := &Worker{imageClient: retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
		calls++
		if calls <= 2 {
			return chatgpt2api.ImageGenerationResponse{}, &chatgpt2api.UpstreamError{StatusCode: 429, Message: "no available image quota"}
		}
		return chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{URL: "https://example.invalid/generated.png"}}}, nil
	}}}

	result, err := worker.generateJobImages(context.Background(), Job{ID: 1, UserID: 2, Prompt: "cat", N: 1})
	if err != nil {
		t.Fatalf("generateJobImages() error = %v", err)
	}
	if calls != 3 {
		t.Fatalf("GenerateImage calls = %d, want 3", calls)
	}
	if len(result.Data) != 1 || result.Data[0].URL == "" {
		t.Fatalf("generateJobImages() result = %#v", result)
	}
}

// TestWorkerGenerateStopsAfterMaxImageQuotaRetries 确认持续无额度不会无限占用队列 worker。
func TestWorkerGenerateStopsAfterMaxImageQuotaRetries(t *testing.T) {
	restoreImageQuotaRetryDelay(t)
	calls := 0
	worker := &Worker{imageClient: retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
		calls++
		return chatgpt2api.ImageGenerationResponse{}, errors.New("no available image quota")
	}}}

	_, err := worker.generateJobImages(context.Background(), Job{ID: 1, UserID: 2, Prompt: "cat", N: 1})
	if err == nil {
		t.Fatal("generateJobImages() expected quota error")
	}
	if calls != maxImageQuotaRetries+1 {
		t.Fatalf("GenerateImage calls = %d, want %d", calls, maxImageQuotaRetries+1)
	}
}

// TestWorkerEditRetriesNoAvailableImageQuota 确认图片编辑链路和文生图使用同一额度重试策略。
func TestWorkerEditRetriesNoAvailableImageQuota(t *testing.T) {
	restoreImageQuotaRetryDelay(t)
	calls := 0
	worker := &Worker{imageClient: retryImageClient{edit: func(context.Context, chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error) {
		calls++
		if calls == 1 {
			return chatgpt2api.ImageGenerationResponse{}, errors.New("wrapped upstream: no available image quota")
		}
		return chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{URL: "https://example.invalid/edited.png"}}}, nil
	}}}

	result, err := worker.editJobImages(context.Background(), Job{
		ID:                  3,
		UserID:              4,
		Prompt:              "edit cat",
		N:                   1,
		SourceImageBytes:    []byte("fake image"),
		SourceImageFilename: "source.png",
	})
	if err != nil {
		t.Fatalf("editJobImages() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("EditImage calls = %d, want 2", calls)
	}
	if len(result.Data) != 1 || result.Data[0].URL == "" {
		t.Fatalf("editJobImages() result = %#v", result)
	}
}

func restoreImageQuotaRetryDelay(t *testing.T) {
	t.Helper()
	original := imageQuotaRetryDelay
	imageQuotaRetryDelay = 0
	t.Cleanup(func() {
		imageQuotaRetryDelay = original
	})
}

func TestWorkerErrorLogKeepsContextAndRedactsSecrets(t *testing.T) {
	var buf bytes.Buffer
	worker := &Worker{logger: log.New(&buf, "", 0)}
	sourceIndex := 2

	worker.logFailedJob(Job{
		ID:                11,
		UserID:            22,
		SessionID:         33,
		GenerationMode:    GenerationModeEdit,
		SourceImageTaskID: 44,
		SourceImageIndex:  &sourceIndex,
		Model:             "gpt-image-1",
		Quality:           "high",
		Size:              "1024x1024",
		N:                 3,
	}, errors.New("upstream failed authorization Bearer real-secret token=abc123 https://example.test/path?auth_key=secret"), "upstream failed")

	line := buf.String()
	for _, want := range []string{
		"image generation job failed",
		"job_id=11",
		"user_id=22",
		"session_id=33",
		"mode=\"edit\"",
		"model=\"gpt-image-1\"",
		"source_task_id=44",
		"source_image_index=2",
		"reason=",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line missing %q: %s", want, line)
		}
	}
	for _, leaked := range []string{"real-secret", "abc123", "auth_key=secret"} {
		if strings.Contains(line, leaked) {
			t.Fatalf("log line leaked secret %q: %s", leaked, line)
		}
	}
}

func TestWorkerErrorMessageRedactsDefaultErrorSummary(t *testing.T) {
	message := workerErrorMessage(errors.New("call failed auth-key sk-test token=abc123"))
	if strings.Contains(message, "sk-test") || strings.Contains(message, "abc123") {
		t.Fatalf("workerErrorMessage leaked secret: %q", message)
	}
	if !strings.Contains(message, "[REDACTED]") {
		t.Fatalf("workerErrorMessage should keep redaction marker: %q", message)
	}
}

func TestWorkerErrorMessageKeepsStableAuthFailureSummary(t *testing.T) {
	if got := workerErrorMessage(chatgpt2api.ErrUnauthorized); got != "chatgpt2api auth key is invalid or unauthorized" {
		t.Fatalf("workerErrorMessage() = %q", got)
	}
}
