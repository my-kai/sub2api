package imagequeue

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/openaiimage"
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

type sourceDownloaderFunc func(context.Context, chatgpt2api.ImageEditRequest) (chatgpt2api.ImageEditRequest, error)

func (fn sourceDownloaderFunc) DownloadEditSourceImage(ctx context.Context, input chatgpt2api.ImageEditRequest) (chatgpt2api.ImageEditRequest, error) {
	return fn(ctx, input)
}

func testWorkerWithChannels(channels []UpstreamChannel, clients map[string]ChannelClient) *Worker {
	return &Worker{channelRunner: &channelRunner{
		loadConfig: func(context.Context) (Config, error) {
			return Config{UpstreamChannels: channels}, nil
		},
		clientFactory: func(channel UpstreamChannel) (ChannelClient, error) {
			client, ok := clients[channel.ID]
			if !ok {
				return nil, errors.New("test channel client is not configured")
			}
			return client, nil
		},
	}}
}

func testChannel(id string, channelType UpstreamChannelType, retryCount int) UpstreamChannel {
	return UpstreamChannel{
		ID:         id,
		Name:       id,
		Type:       channelType,
		Enabled:    true,
		Priority:   defaultUpstreamChannelPriority(0),
		RetryCount: retryCount,
	}
}

func testChannelWithPriority(id string, channelType UpstreamChannelType, retryCount int, priority int) UpstreamChannel {
	channel := testChannel(id, channelType, retryCount)
	channel.Priority = priority
	return channel
}

// TestWorkerGenerateRetriesNoAvailableImageQuota 固化上游临时无额度时的文生图重试行为。
func TestWorkerGenerateRetriesNoAvailableImageQuota(t *testing.T) {
	restoreImageQuotaRetryDelay(t)
	calls := 0
	worker := testWorkerWithChannels([]UpstreamChannel{testChannel("primary", UpstreamChannelTypeChatGPT2API, maxImageQuotaRetries)}, map[string]ChannelClient{"primary": retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
		calls++
		if calls <= 2 {
			return chatgpt2api.ImageGenerationResponse{}, &chatgpt2api.UpstreamError{StatusCode: 429, Message: "no available image quota"}
		}
		return chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{URL: "https://example.invalid/generated.png"}}}, nil
	}}})

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
	worker := testWorkerWithChannels([]UpstreamChannel{testChannel("primary", UpstreamChannelTypeChatGPT2API, maxImageQuotaRetries)}, map[string]ChannelClient{"primary": retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
		calls++
		return chatgpt2api.ImageGenerationResponse{}, errors.New("no available image quota")
	}}})

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
	worker := testWorkerWithChannels([]UpstreamChannel{testChannel("primary", UpstreamChannelTypeChatGPT2API, maxImageQuotaRetries)}, map[string]ChannelClient{"primary": retryImageClient{edit: func(context.Context, chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error) {
		calls++
		if calls == 1 {
			return chatgpt2api.ImageGenerationResponse{}, errors.New("wrapped upstream: no available image quota")
		}
		return chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{URL: "https://example.invalid/edited.png"}}}, nil
	}}})

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

func TestWorkerGenerateSwitchesChannelsAfterRetryCount(t *testing.T) {
	restoreImageQuotaRetryDelay(t)
	primaryCalls := 0
	secondaryCalls := 0
	worker := testWorkerWithChannels([]UpstreamChannel{
		testChannel("primary", UpstreamChannelTypeChatGPT2API, 1),
		testChannel("secondary", UpstreamChannelTypeOpenAI, 0),
	}, map[string]ChannelClient{
		"primary": retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
			primaryCalls++
			return chatgpt2api.ImageGenerationResponse{}, errors.New("temporary upstream failure")
		}},
		"secondary": retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
			secondaryCalls++
			return chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{URL: "https://example.invalid/fallback.png"}}}, nil
		}},
	})

	result, err := worker.generateJobImages(context.Background(), Job{ID: 10, UserID: 20, Prompt: "cat", N: 1})
	if err != nil {
		t.Fatalf("generateJobImages() error = %v", err)
	}
	if primaryCalls != 2 {
		t.Fatalf("primary calls = %d, want 2", primaryCalls)
	}
	if secondaryCalls != 1 {
		t.Fatalf("secondary calls = %d, want 1", secondaryCalls)
	}
	if got := result.Data[0].URL; got != "https://example.invalid/fallback.png" {
		t.Fatalf("fallback URL = %q", got)
	}
}

func TestWorkerGenerateUsesLowerPriorityChannelFirst(t *testing.T) {
	restoreImageQuotaRetryDelay(t)
	order := make([]string, 0, 2)
	worker := testWorkerWithChannels([]UpstreamChannel{
		testChannelWithPriority("fallback", UpstreamChannelTypeOpenAI, 0, 200),
		testChannelWithPriority("primary", UpstreamChannelTypeChatGPT2API, 0, 10),
	}, map[string]ChannelClient{
		"fallback": retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
			order = append(order, "fallback")
			return chatgpt2api.ImageGenerationResponse{}, errors.New("fallback should not run first")
		}},
		"primary": retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
			order = append(order, "primary")
			return chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{URL: "https://example.invalid/primary.png"}}}, nil
		}},
	})

	result, err := worker.generateJobImages(context.Background(), Job{ID: 10, UserID: 20, Prompt: "cat", N: 1})
	if err != nil {
		t.Fatalf("generateJobImages() error = %v", err)
	}
	if len(order) != 1 || order[0] != "primary" {
		t.Fatalf("channel order = %v, want [primary]", order)
	}
	if got := result.Data[0].URL; got != "https://example.invalid/primary.png" {
		t.Fatalf("primary URL = %q", got)
	}
}

func TestWorkerGenerateReturnsAllChannelsFailureSummary(t *testing.T) {
	restoreImageQuotaRetryDelay(t)
	worker := testWorkerWithChannels([]UpstreamChannel{
		testChannel("primary", UpstreamChannelTypeChatGPT2API, 0),
		testChannel("secondary", UpstreamChannelTypeOpenAI, 0),
	}, map[string]ChannelClient{
		"primary": retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
			return chatgpt2api.ImageGenerationResponse{}, errors.New("primary failed auth-key secret")
		}},
		"secondary": retryImageClient{generate: func(context.Context, chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
			return chatgpt2api.ImageGenerationResponse{}, errors.New("secondary failed")
		}},
	})

	_, err := worker.generateJobImages(context.Background(), Job{ID: 10, UserID: 20, Prompt: "cat", N: 1})
	if err == nil {
		t.Fatal("generateJobImages() expected error")
	}
	if !strings.Contains(err.Error(), "primary") || !strings.Contains(err.Error(), "secondary") {
		t.Fatalf("all channels summary missing channel names: %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("all channels summary leaked secret: %v", err)
	}
}

func TestWorkerEditSourceDownloadFailureDoesNotSwitchChannel(t *testing.T) {
	restoreImageQuotaRetryDelay(t)
	calls := 0
	runner := &channelRunner{
		loadConfig: func(context.Context) (Config, error) {
			return Config{UpstreamChannels: []UpstreamChannel{
				testChannel("primary", UpstreamChannelTypeChatGPT2API, 1),
				testChannel("secondary", UpstreamChannelTypeOpenAI, 0),
			}}, nil
		},
		clientFactory: func(UpstreamChannel) (ChannelClient, error) {
			return retryImageClient{edit: func(context.Context, chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error) {
				calls++
				return chatgpt2api.ImageGenerationResponse{Data: []chatgpt2api.ImageGenerationData{{URL: "https://example.invalid/edited.png"}}}, nil
			}}, nil
		},
		sourceDownloader: sourceDownloaderFunc(func(context.Context, chatgpt2api.ImageEditRequest) (chatgpt2api.ImageEditRequest, error) {
			return chatgpt2api.ImageEditRequest{}, fmt.Errorf("%w: source image download failed", chatgpt2api.ErrInvalidRequest)
		}),
	}
	worker := &Worker{channelRunner: runner}

	_, err := worker.editImageWithQuotaRetry(context.Background(), Job{ID: 1, UserID: 2}, chatgpt2api.ImageEditRequest{
		Prompt:   "edit",
		N:        1,
		ImageURL: "https://example.invalid/source.png",
	})
	if err == nil {
		t.Fatal("editImageWithQuotaRetry() expected source download error")
	}
	if calls != 0 {
		t.Fatalf("channel calls = %d, want 0", calls)
	}
	if !errors.Is(err, chatgpt2api.ErrInvalidRequest) {
		t.Fatalf("error should keep invalid request sentinel: %v", err)
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

func TestWorkerErrorMessageHidesUpstreamChannelDetails(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "chatgpt2api auth", err: chatgpt2api.ErrUnauthorized},
		{name: "chatgpt2api upstream", err: &chatgpt2api.UpstreamError{StatusCode: 502, Message: "chatgpt2api upstream exploded"}},
		{name: "openai auth", err: openaiimage.ErrUnauthorized},
		{name: "openai upstream", err: &openaiimage.UpstreamError{StatusCode: 500, Message: "openai upstream exploded"}},
		{name: "all channels", err: &allChannelsFailedError{attempts: []channelAttemptError{{
			channel: testChannel("primary-chatgpt2api", UpstreamChannelTypeChatGPT2API, 0),
			attempt: 1,
			total:   1,
			err:     errors.New("chatgpt2api failed"),
		}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			message := workerErrorMessage(tc.err)
			for _, leaked := range []string{"chatgpt2api", "openai", "upstream", "auth", "primary-chatgpt2api"} {
				if strings.Contains(strings.ToLower(message), leaked) {
					t.Fatalf("workerErrorMessage leaked %q: %q", leaked, message)
				}
			}
			if message != "生图任务执行失败，请稍后重试" {
				t.Fatalf("workerErrorMessage() = %q", message)
			}
		})
	}
}
