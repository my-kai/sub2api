package imagequeue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/openaiimage"
)

// ChannelClient 是 worker 调用具体上游渠道所需的窄接口。
//
// 接口沿用队列内部的请求/响应类型，让 worker 不直接绑定 chatgpt2api 或 OpenAI 官方 client；
// 不同渠道的协议差异由适配器在 custom 包内消化。
type ChannelClient interface {
	GenerateImage(ctx context.Context, input chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error)
	EditImage(ctx context.Context, input chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error)
}

// channelClientFactory 根据单个渠道配置构造实际 client，测试可注入 fake factory 验证编排语义。
type channelClientFactory func(channel UpstreamChannel) (ChannelClient, error)

// channelRunner 在每个批次请求前读取最新渠道配置，并按 priority 从小到大执行重试和切换。
type channelRunner struct {
	loadConfig       func(context.Context) (Config, error)
	clientFactory    channelClientFactory
	sourceDownloader editSourceDownloader
	logger           interface {
		logf(format string, args ...any)
	}
}

type editSourceDownloader interface {
	DownloadEditSourceImage(ctx context.Context, input chatgpt2api.ImageEditRequest) (chatgpt2api.ImageEditRequest, error)
}

type channelAttemptError struct {
	channel UpstreamChannel
	attempt int
	total   int
	err     error
}

// allChannelsFailedError 聚合所有上游失败摘要；落库前会再经过 workerErrorMessage 脱敏截断。
type allChannelsFailedError struct {
	attempts []channelAttemptError
}

func (e *allChannelsFailedError) Error() string {
	if e == nil || len(e.attempts) == 0 {
		return "all image upstream channels failed"
	}
	parts := make([]string, 0, len(e.attempts))
	for _, attempt := range e.attempts {
		parts = append(parts, fmt.Sprintf("%s/%s attempt %d/%d: %s",
			attempt.channel.Type,
			channelDisplayName(attempt.channel),
			attempt.attempt,
			attempt.total,
			sanitizeWorkerErrorForLog(attempt.err),
		))
	}
	return "all image upstream channels failed: " + strings.Join(parts, "; ")
}

func (e *allChannelsFailedError) Unwrap() error {
	if e == nil || len(e.attempts) == 0 {
		return nil
	}
	return e.attempts[len(e.attempts)-1].err
}

// defaultChannelClientFactory 复用进程级 chatgpt2api client 的 HTTP 超时，同时让每个渠道显式传入配置。
func defaultChannelClientFactory(baseChatGPT2API *chatgpt2api.Client, timeout time.Duration) channelClientFactory {
	if baseChatGPT2API != nil && timeout <= 0 {
		timeout = baseChatGPT2API.HTTPTimeout()
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	return func(channel UpstreamChannel) (ChannelClient, error) {
		switch channel.Type {
		case UpstreamChannelTypeChatGPT2API:
			cfg, err := chatgpt2api.NewRuntimeConfig(channel.BaseURL, channel.AuthKey)
			if err != nil {
				return nil, err
			}
			return chatgpt2api.NewClient(cfg.BaseURL, cfg.AuthKey, timeout), nil
		case UpstreamChannelTypeOpenAI:
			client, err := openaiimage.NewClient(channel.BaseURL, channel.AuthKey, timeout, nil)
			if err != nil {
				return nil, err
			}
			return openaiChannelClient{client: client}, nil
		default:
			return nil, fmt.Errorf("%w: upstream channel type is invalid", ErrInvalidInput)
		}
	}
}

// openaiChannelClient 把队列内部请求映射到 OpenAI 官方 Images API 请求。
type openaiChannelClient struct {
	client *openaiimage.Client
}

func (c openaiChannelClient) GenerateImage(ctx context.Context, input chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
	if c.client == nil {
		return chatgpt2api.ImageGenerationResponse{}, openaiimage.ErrNotConfigured
	}
	return c.client.GenerateImage(ctx, openaiimage.GenerateRequest{
		Model:             defaultImageModel,
		Prompt:            input.Prompt,
		N:                 input.N,
		Quality:           input.Quality,
		Size:              input.Size,
		OutputFormat:      input.OutputFormat,
		OutputCompression: input.OutputCompression,
	})
}

func (c openaiChannelClient) EditImage(ctx context.Context, input chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error) {
	if c.client == nil {
		return chatgpt2api.ImageGenerationResponse{}, openaiimage.ErrNotConfigured
	}
	return c.client.EditImage(ctx, openaiimage.EditRequest{
		Model:             defaultImageModel,
		Prompt:            input.Prompt,
		N:                 input.N,
		Quality:           input.Quality,
		Size:              input.Size,
		OutputFormat:      input.OutputFormat,
		OutputCompression: input.OutputCompression,
		ImageBytes:        input.ImageBytes,
		ImageFilename:     input.ImageFilename,
		ImageContentType:  input.ImageContentType,
	})
}

func newChannelRunner(store *Store, baseChatGPT2API *chatgpt2api.Client, logger interface {
	logf(format string, args ...any)
}) *channelRunner {
	timeout := time.Duration(0)
	if baseChatGPT2API != nil {
		timeout = baseChatGPT2API.HTTPTimeout()
	}
	return &channelRunner{
		loadConfig: func(ctx context.Context) (Config, error) {
			if store == nil {
				return Config{}, fmt.Errorf("%w: image queue store is not configured", ErrInvalidInput)
			}
			return store.GetConfig(ctx)
		},
		clientFactory:    defaultChannelClientFactory(baseChatGPT2API, timeout),
		sourceDownloader: baseChatGPT2API,
		logger:           logger,
	}
}

func (r *channelRunner) GenerateImage(ctx context.Context, job Job, request chatgpt2api.ImageGenerationRequest) (chatgpt2api.ImageGenerationResponse, error) {
	return r.run(ctx, job, "generate", func(ctx context.Context, client ChannelClient) (chatgpt2api.ImageGenerationResponse, error) {
		return client.GenerateImage(ctx, request)
	})
}

func (r *channelRunner) EditImage(ctx context.Context, job Job, request chatgpt2api.ImageEditRequest) (chatgpt2api.ImageGenerationResponse, error) {
	prepared, err := r.prepareEditSource(ctx, request)
	if err != nil {
		return chatgpt2api.ImageGenerationResponse{}, err
	}
	return r.run(ctx, job, "edit", func(ctx context.Context, client ChannelClient) (chatgpt2api.ImageGenerationResponse, error) {
		return client.EditImage(ctx, prepared)
	})
}

func (r *channelRunner) prepareEditSource(ctx context.Context, request chatgpt2api.ImageEditRequest) (chatgpt2api.ImageEditRequest, error) {
	if strings.TrimSpace(request.ImageURL) == "" {
		return request, nil
	}
	if r == nil || r.sourceDownloader == nil {
		return chatgpt2api.ImageEditRequest{}, fmt.Errorf("%w: source image downloader is not configured", chatgpt2api.ErrInvalidRequest)
	}
	return r.sourceDownloader.DownloadEditSourceImage(ctx, request)
}

func (r *channelRunner) run(ctx context.Context, job Job, operation string, call func(context.Context, ChannelClient) (chatgpt2api.ImageGenerationResponse, error)) (chatgpt2api.ImageGenerationResponse, error) {
	if r == nil || r.loadConfig == nil {
		return chatgpt2api.ImageGenerationResponse{}, chatgpt2api.ErrNotConfigured
	}
	cfg, err := r.loadConfig(ctx)
	if err != nil {
		return chatgpt2api.ImageGenerationResponse{}, err
	}
	channels := enabledUpstreamChannels(cfg.UpstreamChannels)
	if len(channels) == 0 {
		return chatgpt2api.ImageGenerationResponse{}, chatgpt2api.ErrNotConfigured
	}
	attempts := make([]channelAttemptError, 0)
	for _, channel := range channels {
		client, err := r.clientForChannel(channel)
		if err != nil {
			attempts = append(attempts, channelAttemptError{channel: channel, attempt: 1, total: 1, err: err})
			r.logAttempt(job, operation, channel, 1, 1, err)
			continue
		}
		totalAttempts := 1 + channel.RetryCount
		for attempt := 1; attempt <= totalAttempts; attempt++ {
			result, err := call(ctx, client)
			if err == nil {
				return result, nil
			}
			if !isUpstreamSwitchableError(err) {
				return chatgpt2api.ImageGenerationResponse{}, err
			}
			attempts = append(attempts, channelAttemptError{channel: channel, attempt: attempt, total: totalAttempts, err: err})
			r.logAttempt(job, operation, channel, attempt, totalAttempts, err)
			if attempt < totalAttempts {
				if err := waitImageQuotaRetry(ctx); err != nil {
					return chatgpt2api.ImageGenerationResponse{}, err
				}
			}
		}
	}
	return chatgpt2api.ImageGenerationResponse{}, &allChannelsFailedError{attempts: attempts}
}

func (r *channelRunner) clientForChannel(channel UpstreamChannel) (ChannelClient, error) {
	if r == nil || r.clientFactory == nil {
		return nil, chatgpt2api.ErrNotConfigured
	}
	return r.clientFactory(channel)
}

func (r *channelRunner) logAttempt(job Job, operation string, channel UpstreamChannel, attempt int, total int, err error) {
	if r == nil || r.logger == nil || err == nil {
		return
	}
	r.logger.logf(
		"image generation upstream attempt failed: job_id=%d user_id=%d mode=%q channel_type=%q channel_name=%q attempt=%d/%d reason=%q",
		job.ID,
		job.UserID,
		operation,
		channel.Type,
		channelDisplayName(channel),
		attempt,
		total,
		sanitizeWorkerErrorForLog(err),
	)
}

func enabledUpstreamChannels(channels []UpstreamChannel) []UpstreamChannel {
	normalized := normalizeUpstreamChannels(channels)
	enabled := make([]UpstreamChannel, 0, len(normalized))
	for _, channel := range normalized {
		if !channel.Enabled {
			continue
		}
		enabled = append(enabled, channel)
	}
	return enabled
}

func channelDisplayName(channel UpstreamChannel) string {
	if name := strings.TrimSpace(channel.Name); name != "" {
		return name
	}
	return defaultUpstreamChannelName(channel.Type)
}

// isUpstreamSwitchableError 只把实际上游调用失败纳入渠道重试/切换。
//
// 来源图下载、请求校验、DB 读写和 context 取消都不应在这里被吞掉，否则会误把本地问题包装成
// “换个上游可能好”的错误，造成重复请求和更难排查的失败摘要。
func isUpstreamSwitchableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, chatgpt2api.ErrInvalidRequest) || errors.Is(err, openaiimage.ErrInvalidRequest) {
		return false
	}
	return true
}
