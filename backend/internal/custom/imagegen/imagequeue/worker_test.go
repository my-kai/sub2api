package imagequeue

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
)

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
