package chatgpt2api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultImageModel       = "gpt-image-2"
	maxImageCount           = 10
	maxPromptLength         = 8000
	maxImageBodySize        = 64 << 20
	minSourceImageTimeout   = 10 * time.Minute
	imageResponseURL        = "url"
	defaultImageFilename    = "source.png"
	defaultImageContentType = "image/png"
)

var (
	// ErrNotConfigured 表示当前进程没有配置 chatgpt2api 上游或鉴权密钥。
	ErrNotConfigured = errors.New("chatgpt2api is not configured")
	// ErrInvalidRequest 表示前端请求没有满足图片生成接口的基本约束。
	ErrInvalidRequest = errors.New("invalid image generation request")
	// ErrUnauthorized 表示 chatgpt2api 拒绝了当前 auth-key。
	ErrUnauthorized = errors.New("chatgpt2api auth key is invalid or unauthorized")
	// ErrBadResponse 表示上游响应无法按预期解析。
	ErrBadResponse = errors.New("chatgpt2api returned an unexpected response")
)

// UpstreamError 保留 chatgpt2api 返回的业务错误，供 handler 映射为可读提示。
type UpstreamError struct {
	StatusCode int
	Message    string
}

// Error 返回已脱敏的上游错误文案。
func (e *UpstreamError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("chatgpt2api upstream error: status %d", e.StatusCode)
	}
	return e.Message
}

// Client 负责服务端调用 chatgpt2api 图片开放接口。
//
// authKey 只保存在后端进程内，不进入前端构建产物、浏览器请求或可复制诊断信息。
type Client struct {
	baseURL              *url.URL
	authKey              string
	httpClient           *http.Client
	sourceDownloadClient *http.Client
	configLoader         ConfigLoader
}

// RuntimeConfig 是 chatgpt2api 单次请求使用的运行期配置。
type RuntimeConfig struct {
	BaseURL *url.URL
	AuthKey string
}

// ConfigLoader 允许调用方在每次请求前读取 DB 中的最新 chatgpt2api 配置。
type ConfigLoader func(ctx context.Context) (RuntimeConfig, error)

// NewClient 创建 chatgpt2api 客户端。
//
// baseURL 或 authKey 为空时仍允许构造对象，具体请求会返回 ErrNotConfigured，避免未启用生图功能时阻断
// 其他扩展服务启动。
func NewClient(baseURL *url.URL, authKey string, timeout time.Duration) *Client {
	var copied *url.URL
	if baseURL != nil {
		value := *baseURL
		copied = &value
	}
	return &Client{
		baseURL: copied,
		authKey: strings.TrimSpace(authKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		sourceDownloadClient: newSourceDownloadClient(sourceImageDownloadTimeout(timeout)),
	}
}

func sourceImageDownloadTimeout(timeout time.Duration) time.Duration {
	if timeout < minSourceImageTimeout {
		return minSourceImageTimeout
	}
	return timeout
}

func newSourceDownloadClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// 来源图下载最容易卡在 DNS / TCP 连接阶段；默认 Transport 的 30 秒拨号超时会早于
	// IMAGE_CLIENT_TIMEOUT 触发，这里显式拉齐，避免大图编辑还没进入上游生图就失败。
	transport.DialContext = (&net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}).DialContext
	transport.TLSHandshakeTimeout = timeout
	transport.ResponseHeaderTimeout = timeout
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

// WithConfigLoader 让客户端优先使用运行期配置；loader 返回空配置时回退到构造参数。
func (c *Client) WithConfigLoader(loader ConfigLoader) *Client {
	if c == nil {
		return nil
	}
	c.configLoader = loader
	return c
}

// NewRuntimeConfig 从管理员配置中的字符串构造可请求配置。
func NewRuntimeConfig(rawBaseURL string, rawAuthKey string) (RuntimeConfig, error) {
	trimmedBaseURL := strings.TrimSpace(rawBaseURL)
	cfg := RuntimeConfig{AuthKey: strings.TrimSpace(rawAuthKey)}
	if trimmedBaseURL == "" {
		return cfg, nil
	}
	parsed, err := url.Parse(trimmedBaseURL)
	if err != nil {
		return RuntimeConfig{}, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return RuntimeConfig{}, fmt.Errorf("chatgpt2api base_url must use http or https scheme")
	}
	if parsed.Host == "" {
		return RuntimeConfig{}, fmt.Errorf("chatgpt2api base_url must include host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	cfg.BaseURL = parsed
	return cfg, nil
}

// Configured 表示当前客户端具备调用 chatgpt2api 的最低配置。
func (c *Client) Configured() bool {
	cfg, err := c.runtimeConfig(context.Background())
	return err == nil && cfg.configured()
}

// GenerateImage 调用 OpenAI 兼容的 /v1/images/generations 文生图接口。
func (c *Client) GenerateImage(ctx context.Context, input ImageGenerationRequest) (ImageGenerationResponse, error) {
	cfg, err := c.runtimeConfig(ctx)
	if err != nil {
		return ImageGenerationResponse{}, err
	}
	if !cfg.configured() {
		return ImageGenerationResponse{}, ErrNotConfigured
	}

	request := normalizeGenerationRequest(input)
	if err := validateGenerationRequest(request); err != nil {
		return ImageGenerationResponse{}, err
	}

	var result ImageGenerationResponse
	if err := c.postJSON(ctx, cfg, "/v1/images/generations", request, &result); err != nil {
		return ImageGenerationResponse{}, err
	}
	if result.Data == nil {
		result.Data = []ImageGenerationData{}
	}
	return result, nil
}

// EditImage 调用 OpenAI 兼容的 /v1/images/edits 图片编辑接口。
func (c *Client) EditImage(ctx context.Context, input ImageEditRequest) (ImageGenerationResponse, error) {
	cfg, err := c.runtimeConfig(ctx)
	if err != nil {
		return ImageGenerationResponse{}, err
	}
	if !cfg.configured() {
		return ImageGenerationResponse{}, ErrNotConfigured
	}

	request := normalizeEditRequest(input)
	if err := validateEditRequest(request); err != nil {
		return ImageGenerationResponse{}, err
	}

	var result ImageGenerationResponse
	if request.ImageURL != "" {
		downloaded, err := c.downloadEditSourceImage(ctx, request)
		if err != nil {
			return ImageGenerationResponse{}, err
		}
		request = downloaded
	}
	if err := c.postMultipart(ctx, cfg, "/v1/images/edits", request, &result); err != nil {
		return ImageGenerationResponse{}, err
	}
	if result.Data == nil {
		result.Data = []ImageGenerationData{}
	}
	return result, nil
}

// Models 读取 chatgpt2api 当前暴露的图片模型列表。
func (c *Client) Models(ctx context.Context) (ModelsResponse, error) {
	cfg, err := c.runtimeConfig(ctx)
	if err != nil {
		return ModelsResponse{}, err
	}
	if !cfg.configured() {
		return ModelsResponse{}, ErrNotConfigured
	}

	var result ModelsResponse
	if err := c.getJSON(ctx, cfg, "/v1/models", &result); err != nil {
		return ModelsResponse{}, err
	}
	if result.Data == nil {
		result.Data = []Model{}
	}
	return result, nil
}

func normalizeGenerationRequest(input ImageGenerationRequest) ImageGenerationRequest {
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = defaultImageModel
	}
	count := input.N
	if count == 0 {
		count = 1
	}
	return ImageGenerationRequest{
		Model:          model,
		Prompt:         strings.TrimSpace(input.Prompt),
		N:              count,
		Quality:        normalizeGenerationQuality(input.Quality),
		Size:           normalizeGenerationSize(input.Size),
		ResponseFormat: imageResponseURL,
	}
}

func normalizeEditRequest(input ImageEditRequest) ImageEditRequest {
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = defaultImageModel
	}
	count := input.N
	if count == 0 {
		count = 1
	}
	filename := strings.TrimSpace(input.ImageFilename)
	if filename == "" {
		filename = defaultImageFilename
	}
	return ImageEditRequest{
		Model:            model,
		Prompt:           strings.TrimSpace(input.Prompt),
		N:                count,
		Quality:          normalizeGenerationQuality(input.Quality),
		Size:             normalizeGenerationSize(input.Size),
		ResponseFormat:   imageResponseURL,
		ImageURL:         strings.TrimSpace(input.ImageURL),
		ImageBytes:       input.ImageBytes,
		ImageFilename:    filename,
		ImageContentType: normalizeImageContentType(input.ImageContentType),
	}
}

func normalizeGenerationQuality(value string) string {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "auto", "low", "medium", "high":
		return trimmed
	default:
		return ""
	}
}

func normalizeGenerationSize(value string) string {
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

// normalizeImageContentType 只透传明确的图片 MIME，避免把任意响应头写进 multipart 元数据。
func normalizeImageContentType(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(trimmed, "image/") {
		return trimmed
	}
	return defaultImageContentType
}

func validateGenerationRequest(input ImageGenerationRequest) error {
	if input.Prompt == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalidRequest)
	}
	if len(input.Prompt) > maxPromptLength {
		return fmt.Errorf("%w: prompt is too long", ErrInvalidRequest)
	}
	if input.N < 1 || input.N > maxImageCount {
		return fmt.Errorf("%w: n must be between 1 and %d", ErrInvalidRequest, maxImageCount)
	}
	return nil
}

func validateEditRequest(input ImageEditRequest) error {
	if input.Prompt == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalidRequest)
	}
	if len(input.Prompt) > maxPromptLength {
		return fmt.Errorf("%w: prompt is too long", ErrInvalidRequest)
	}
	if input.N < 1 || input.N > maxImageCount {
		return fmt.Errorf("%w: n must be between 1 and %d", ErrInvalidRequest, maxImageCount)
	}
	if strings.TrimSpace(input.ImageURL) == "" && len(input.ImageBytes) == 0 {
		return fmt.Errorf("%w: image is required", ErrInvalidRequest)
	}
	if strings.TrimSpace(input.ImageURL) != "" {
		if err := validateImageURL(input.ImageURL); err != nil {
			return err
		}
	}
	if len(input.ImageBytes) > maxImageBodySize {
		return fmt.Errorf("%w: image is too large", ErrInvalidRequest)
	}
	return nil
}

// validateImageURL 只允许 http/https 图片链接进入 2api 编辑接口。
func validateImageURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: image url is invalid", ErrInvalidRequest)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return fmt.Errorf("%w: image url scheme is invalid", ErrInvalidRequest)
	}
}

func (c *Client) getJSON(ctx context.Context, cfg RuntimeConfig, path string, output any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL(cfg.BaseURL, path), nil)
	if err != nil {
		return fmt.Errorf("build chatgpt2api request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AuthKey)
	return c.doJSON(req, output)
}

func (c *Client) postJSON(ctx context.Context, cfg RuntimeConfig, path string, input any, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode chatgpt2api request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(cfg.BaseURL, path), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build chatgpt2api request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.AuthKey)
	return c.doJSON(req, output)
}

func (c *Client) postMultipart(ctx context.Context, cfg RuntimeConfig, path string, input ImageEditRequest, output any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writeMultipartField(writer, "model", input.Model); err != nil {
		return err
	}
	if err := writeMultipartField(writer, "prompt", input.Prompt); err != nil {
		return err
	}
	if err := writeMultipartField(writer, "n", fmt.Sprintf("%d", input.N)); err != nil {
		return err
	}
	if err := writeMultipartField(writer, "response_format", input.ResponseFormat); err != nil {
		return err
	}
	if input.Quality != "" {
		if err := writeMultipartField(writer, "quality", input.Quality); err != nil {
			return err
		}
	}
	if input.Size != "" {
		if err := writeMultipartField(writer, "size", input.Size); err != nil {
			return err
		}
	}
	// image 字段名保持 OpenAI 兼容约定；显式写入 MIME，避免上游按空 content-type 兜底失败。
	fileWriter, err := createImageFormFile(writer, "image", input.ImageFilename, input.ImageContentType)
	if err != nil {
		return fmt.Errorf("build chatgpt2api multipart image field: %w", err)
	}
	if _, err := fileWriter.Write(input.ImageBytes); err != nil {
		return fmt.Errorf("write chatgpt2api multipart image: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close chatgpt2api multipart body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(cfg.BaseURL, path), &body)
	if err != nil {
		return fmt.Errorf("build chatgpt2api request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.AuthKey)
	return c.doJSON(req, output)
}

func writeMultipartField(writer *multipart.Writer, key string, value string) error {
	if err := writer.WriteField(key, value); err != nil {
		return fmt.Errorf("write chatgpt2api multipart field %s: %w", key, err)
	}
	return nil
}

func (c *Client) doJSON(req *http.Request, output any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call chatgpt2api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBodySize))
	if err != nil {
		return fmt.Errorf("read chatgpt2api response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &UpstreamError{StatusCode: resp.StatusCode, Message: extractErrorMessage(body, resp.StatusCode)}
	}
	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	return nil
}

func (c *Client) endpointURL(path string) string {
	cfg, err := c.staticRuntimeConfig()
	if err != nil || cfg.BaseURL == nil {
		return path
	}
	return endpointURL(cfg.BaseURL, path)
}

func endpointURL(baseURL *url.URL, path string) string {
	u := *baseURL
	basePath := strings.TrimRight(u.Path, "/")
	requestPath := path
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(requestPath, "/v1/") {
		// chatgpt2api 文档里 API 地址常写成 http://host/v1；这里兼容该配置，避免拼出 /v1/v1。
		requestPath = strings.TrimPrefix(requestPath, "/v1")
	}
	u.Path = basePath + requestPath
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (c *Client) runtimeConfig(ctx context.Context) (RuntimeConfig, error) {
	if c == nil {
		return RuntimeConfig{}, nil
	}
	if c.configLoader != nil {
		cfg, err := c.configLoader(ctx)
		if err != nil {
			return RuntimeConfig{}, err
		}
		if cfg.BaseURL != nil || strings.TrimSpace(cfg.AuthKey) != "" {
			cfg.AuthKey = strings.TrimSpace(cfg.AuthKey)
			return cfg, nil
		}
	}
	return c.staticRuntimeConfig()
}

func (c *Client) staticRuntimeConfig() (RuntimeConfig, error) {
	if c == nil {
		return RuntimeConfig{}, nil
	}
	var copied *url.URL
	if c.baseURL != nil {
		value := *c.baseURL
		copied = &value
	}
	return RuntimeConfig{BaseURL: copied, AuthKey: strings.TrimSpace(c.authKey)}, nil
}

func (cfg RuntimeConfig) configured() bool {
	return cfg.BaseURL != nil && strings.TrimSpace(cfg.AuthKey) != ""
}

func extractErrorMessage(body []byte, status int) string {
	var envelope struct {
		Message string `json:"message"`
		Error   any    `json:"error"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		if message := strings.TrimSpace(envelope.Message); message != "" {
			return message
		}
		if message := strings.TrimSpace(envelope.Detail); message != "" {
			return message
		}
		if value, ok := envelope.Error.(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
		if value, ok := envelope.Error.(map[string]any); ok {
			if message, ok := value["message"].(string); ok && strings.TrimSpace(message) != "" {
				return message
			}
		}
	}
	return fmt.Sprintf("status %d", status)
}
