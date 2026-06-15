package openaiimage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/custom/imagegen/chatgpt2api"
)

const (
	defaultBaseURL       = "https://api.openai.com"
	defaultImageModel    = "gpt-image-2"
	maxImageResponseBody = 128 << 20
)

var (
	// ErrNotConfigured 表示 OpenAI 官方图片渠道缺少 base_url 或 auth_key。
	ErrNotConfigured = errors.New("openai image channel is not configured")
	// ErrInvalidRequest 表示请求在进入官方上游前不满足基本约束。
	ErrInvalidRequest = errors.New("invalid openai image request")
	// ErrUnauthorized 表示官方上游拒绝当前 Bearer token。
	ErrUnauthorized = errors.New("openai image auth key is invalid or unauthorized")
	// ErrBadResponse 表示官方上游响应无法归一化为本项目任务结果。
	ErrBadResponse = errors.New("openai image returned an unexpected response")
)

// UpstreamError 保留官方渠道 HTTP 非 2xx 的状态与脱敏消息，供渠道 runner 做失败摘要。
type UpstreamError struct {
	StatusCode int
	Message    string
}

// Error 返回官方渠道上游错误摘要。
func (e *UpstreamError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Sprintf("openai image upstream error: status %d", e.StatusCode)
	}
	return e.Message
}

// RuntimeConfig 是单个 OpenAI 官方渠道的运行期配置。
type RuntimeConfig struct {
	BaseURL *url.URL
	AuthKey string
}

// Client 负责调用 OpenAI 官方 Images API 并把 b64_json 归一化为本项目图片 URL。
type Client struct {
	baseURL      *url.URL
	authKey      string
	httpClient   *http.Client
	assetBuilder AssetURLBuilder
}

// NewClient 创建 OpenAI 官方图片客户端。
func NewClient(rawBaseURL string, authKey string, timeout time.Duration, assetBuilder AssetURLBuilder) (*Client, error) {
	cfg, err := NewRuntimeConfig(rawBaseURL, authKey)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	if assetBuilder == nil {
		assetBuilder = DataURLAssetBuilder{}
	}
	return &Client{
		baseURL:      cfg.BaseURL,
		authKey:      cfg.AuthKey,
		httpClient:   &http.Client{Timeout: timeout},
		assetBuilder: assetBuilder,
	}, nil
}

// NewRuntimeConfig 从管理员渠道配置构造可请求配置。
func NewRuntimeConfig(rawBaseURL string, rawAuthKey string) (RuntimeConfig, error) {
	baseURL := strings.TrimSpace(rawBaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return RuntimeConfig{}, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return RuntimeConfig{}, fmt.Errorf("openai image base_url must use http or https scheme")
	}
	if parsed.Host == "" {
		return RuntimeConfig{}, fmt.Errorf("openai image base_url must include host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return RuntimeConfig{BaseURL: parsed, AuthKey: strings.TrimSpace(rawAuthKey)}, nil
}

// GenerateImage 调用官方 /v1/images/generations 文生图接口。
func (c *Client) GenerateImage(ctx context.Context, input GenerateRequest) (chatgpt2api.ImageGenerationResponse, error) {
	if c == nil || c.baseURL == nil || strings.TrimSpace(c.authKey) == "" {
		return chatgpt2api.ImageGenerationResponse{}, ErrNotConfigured
	}
	request := normalizeGenerateRequest(input)
	if err := validateGenerateRequest(request); err != nil {
		return chatgpt2api.ImageGenerationResponse{}, err
	}
	var result Response
	if err := c.postJSON(ctx, "/v1/images/generations", request, &result); err != nil {
		return chatgpt2api.ImageGenerationResponse{}, err
	}
	return c.normalizeResponse(result)
}

// EditImage 调用官方 /v1/images/edits 图片编辑接口。
func (c *Client) EditImage(ctx context.Context, input EditRequest) (chatgpt2api.ImageGenerationResponse, error) {
	if c == nil || c.baseURL == nil || strings.TrimSpace(c.authKey) == "" {
		return chatgpt2api.ImageGenerationResponse{}, ErrNotConfigured
	}
	request := normalizeEditRequest(input)
	if err := validateEditRequest(request); err != nil {
		return chatgpt2api.ImageGenerationResponse{}, err
	}
	var result Response
	if err := c.postMultipart(ctx, "/v1/images/edits", request, &result); err != nil {
		return chatgpt2api.ImageGenerationResponse{}, err
	}
	return c.normalizeResponse(result)
}

func normalizeGenerateRequest(input GenerateRequest) GenerateRequest {
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = defaultImageModel
	}
	count := input.N
	if count == 0 {
		count = 1
	}
	return GenerateRequest{
		Model:             model,
		Prompt:            strings.TrimSpace(input.Prompt),
		N:                 count,
		Quality:           strings.TrimSpace(input.Quality),
		Size:              strings.TrimSpace(input.Size),
		OutputFormat:      strings.TrimSpace(input.OutputFormat),
		OutputCompression: input.OutputCompression,
	}
}

func normalizeEditRequest(input EditRequest) EditRequest {
	request := EditRequest{
		Model:             normalizeGenerateRequest(GenerateRequest{Model: input.Model}).Model,
		Prompt:            strings.TrimSpace(input.Prompt),
		N:                 input.N,
		Quality:           strings.TrimSpace(input.Quality),
		Size:              strings.TrimSpace(input.Size),
		OutputFormat:      strings.TrimSpace(input.OutputFormat),
		OutputCompression: input.OutputCompression,
		ImageBytes:        input.ImageBytes,
		ImageFilename:     strings.TrimSpace(input.ImageFilename),
		ImageContentType:  normalizeImageContentType(input.ImageContentType),
	}
	if request.N == 0 {
		request.N = 1
	}
	if request.ImageFilename == "" {
		request.ImageFilename = "source.png"
	}
	return request
}

func validateGenerateRequest(input GenerateRequest) error {
	if strings.TrimSpace(input.Prompt) == "" {
		return fmt.Errorf("%w: prompt is required", ErrInvalidRequest)
	}
	if input.N < 1 || input.N > 10 {
		return fmt.Errorf("%w: n must be between 1 and 10", ErrInvalidRequest)
	}
	return nil
}

func validateEditRequest(input EditRequest) error {
	if err := validateGenerateRequest(GenerateRequest{Prompt: input.Prompt, N: input.N}); err != nil {
		return err
	}
	if len(input.ImageBytes) == 0 {
		return fmt.Errorf("%w: image is required", ErrInvalidRequest)
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, path string, input any, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode openai image request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(c.baseURL, path), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build openai image request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.authKey)
	return c.doJSON(req, output)
}

func (c *Client) postMultipart(ctx context.Context, path string, input EditRequest, output any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"model":  input.Model,
		"prompt": input.Prompt,
		"n":      fmt.Sprintf("%d", input.N),
	} {
		if err := writeMultipartField(writer, key, value); err != nil {
			return err
		}
	}
	for key, value := range map[string]string{
		"quality":            input.Quality,
		"size":               input.Size,
		"output_format":      input.OutputFormat,
		"output_compression": compressionField(input.OutputCompression),
	} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if err := writeMultipartField(writer, key, value); err != nil {
			return err
		}
	}
	// 官方编辑接口按 image[] 接收一个或多个来源图；当前队列只保存单图快照，先按单元素数组提交。
	fileWriter, err := createImageFormFile(writer, "image[]", input.ImageFilename, input.ImageContentType)
	if err != nil {
		return fmt.Errorf("build openai image multipart field: %w", err)
	}
	if _, err := fileWriter.Write(input.ImageBytes); err != nil {
		return fmt.Errorf("write openai image multipart body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close openai image multipart body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(c.baseURL, path), &body)
	if err != nil {
		return fmt.Errorf("build openai image request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.authKey)
	return c.doJSON(req, output)
}

func (c *Client) doJSON(req *http.Request, output any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call openai image: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageResponseBody+1))
	if err != nil {
		return fmt.Errorf("read openai image response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &UpstreamError{StatusCode: resp.StatusCode, Message: extractErrorMessage(body, resp.StatusCode)}
	}
	if len(body) > maxImageResponseBody {
		return fmt.Errorf("%w: response body is too large", ErrBadResponse)
	}
	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	return nil
}

func (c *Client) normalizeResponse(result Response) (chatgpt2api.ImageGenerationResponse, error) {
	data := make([]chatgpt2api.ImageGenerationData, 0, len(result.Data))
	for _, item := range result.Data {
		imageURL := strings.TrimSpace(item.URL)
		if imageURL == "" && strings.TrimSpace(item.B64JSON) != "" {
			url, err := c.assetBuilder.URLForBase64Image(item.B64JSON)
			if err != nil {
				return chatgpt2api.ImageGenerationResponse{}, err
			}
			imageURL = url
		}
		if imageURL == "" {
			return chatgpt2api.ImageGenerationResponse{}, fmt.Errorf("%w: image result has no url or b64_json", ErrBadResponse)
		}
		data = append(data, chatgpt2api.ImageGenerationData{
			URL:           imageURL,
			RevisedPrompt: item.RevisedPrompt,
		})
	}
	if data == nil {
		data = []chatgpt2api.ImageGenerationData{}
	}
	return chatgpt2api.ImageGenerationResponse{Created: result.Created, Data: data}, nil
}

func endpointURL(baseURL *url.URL, path string) string {
	u := *baseURL
	basePath := strings.TrimRight(u.Path, "/")
	requestPath := path
	if strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(requestPath, "/v1/") {
		requestPath = strings.TrimPrefix(requestPath, "/v1")
	}
	u.Path = basePath + requestPath
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func writeMultipartField(writer *multipart.Writer, key string, value string) error {
	if err := writer.WriteField(key, value); err != nil {
		return fmt.Errorf("write openai image multipart field %s: %w", key, err)
	}
	return nil
}

func createImageFormFile(writer *multipart.Writer, fieldName string, filename string, contentType string) (io.Writer, error) {
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     fieldName,
		"filename": filename,
	}))
	header.Set("Content-Type", normalizeImageContentType(contentType))
	return writer.CreatePart(header)
}

func normalizeImageContentType(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(trimmed, "image/") {
		return trimmed
	}
	return defaultAssetContentType
}

func compressionField(value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
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
