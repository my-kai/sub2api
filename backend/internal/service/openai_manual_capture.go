package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

const (
	// manualCaptureBodyLimit bounds diagnostic responses returned to the admin browser.
	manualCaptureBodyLimit int64 = 2 << 20
	// manualCaptureHeaderLimit bounds the serialized response headers returned to the admin browser.
	manualCaptureHeaderLimit int64 = 256 << 10
)

// OpenAIManualCaptureResult contains the raw upstream response from one diagnostic request.
type OpenAIManualCaptureResult struct {
	StatusCode       int                 `json:"status_code"`
	ResponseHeaders  map[string][]string `json:"response_headers"`
	ResponseBody     string              `json:"response_body"`
	HeadersTruncated bool                `json:"headers_truncated"`
	BodyTruncated    bool                `json:"body_truncated"`
}

// CaptureOpenAIChatCompletions sends one non-streaming Chat Completions semantic request
// through the existing OAuth Responses adapter and returns the unconverted upstream reply.
// The caller must provide an OpenAI OAuth account; token refresh, proxy selection,
// authentication headers, and the shared upstream transport remain centralized here.
func (s *OpenAIGatewayService) CaptureOpenAIChatCompletions(ctx context.Context, c *gin.Context, account *Account, model string) (*OpenAIManualCaptureResult, error) {
	if s == nil || s.httpUpstream == nil || s.openAITokenProvider == nil {
		return nil, errors.New("openai gateway capture dependencies are unavailable")
	}
	if c == nil || c.Request == nil {
		return nil, errors.New("capture request context is unavailable")
	}
	if account == nil || !account.IsOpenAIOAuth() {
		return nil, errors.New("an OpenAI OAuth account is required")
	}
	if model == "" {
		return nil, errors.New("capture model is required")
	}

	if _, err := s.prepareCodexAccountIdentitySource(ctx, c, account); err != nil {
		return nil, fmt.Errorf("prepare capture account identity: %w", err)
	}

	chatRequest := apicompat.ChatCompletionsRequest{
		Model: model,
		Messages: []apicompat.ChatMessage{{
			Role:    "user",
			Content: json.RawMessage(`"hi"`),
		}},
		Stream: false,
	}
	responsesRequest, err := apicompat.ChatCompletionsToResponses(&chatRequest)
	if err != nil {
		return nil, fmt.Errorf("convert capture request: %w", err)
	}
	responsesRequest.Model = normalizeOpenAIModelForUpstream(account, model)
	responsesRequest.Stream = false
	responsesRequest.Store = manualCaptureBoolPtr(false)
	responsesBody, err := json.Marshal(responsesRequest)
	if err != nil {
		return nil, fmt.Errorf("marshal capture request: %w", err)
	}
	var requestBody map[string]any
	if err := json.Unmarshal(responsesBody, &requestBody); err != nil {
		return nil, fmt.Errorf("prepare capture request: %w", err)
	}
	codexResult := applyCodexOAuthTransformWithOptions(requestBody, codexOAuthTransformOptions{SkipDefaultInstructions: false})
	if codexResult.Error != nil {
		return nil, fmt.Errorf("apply OAuth request transform: %w", codexResult.Error)
	}
	applyCodexAccountIdentityClientMetadataMap(requestBody, codexAccountIdentitySource(c, account), getAPIKeyIDFromContext(c))
	// The normal gateway adapter intentionally forces streaming for client compatibility;
	// this diagnostic endpoint explicitly requests the raw non-streaming response instead.
	requestBody["stream"] = false
	responsesBody, err = json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal transformed capture request: %w", err)
	}
	responsesBody, err = s.applyOpenAIFastPolicyToBody(ctx, account, responsesRequest.Model, responsesBody)
	if err != nil {
		return nil, fmt.Errorf("apply capture model policy: %w", err)
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get OAuth access token: %w", err)
	}
	upstreamRequest, err := s.buildUpstreamRequest(ctx, c, account, responsesBody, token, false, "", false)
	if err != nil {
		return nil, fmt.Errorf("build capture upstream request: %w", err)
	}
	upstreamRequest.Header.Set("Accept", "application/json")
	response, err := s.doOpenAIUpstream(upstreamRequest, accountProxyURL(account), account)
	if err != nil {
		return nil, fmt.Errorf("send capture upstream request: %w", err)
	}
	defer response.Body.Close()

	headers, headersTruncated := boundedResponseHeaders(response.Header)
	body, bodyTruncated, err := readBoundedResponseBody(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read capture upstream response: %w", err)
	}
	return &OpenAIManualCaptureResult{
		StatusCode:       response.StatusCode,
		ResponseHeaders:  headers,
		ResponseBody:     string(body),
		HeadersTruncated: headersTruncated,
		BodyTruncated:    bodyTruncated,
	}, nil
}

func manualCaptureBoolPtr(value bool) *bool { return &value }

func accountProxyURL(account *Account) string {
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

func boundedResponseHeaders(headers http.Header) (map[string][]string, bool) {
	result := make(map[string][]string, len(headers))
	used := int64(2)
	truncated := false
	for key, values := range headers {
		for _, value := range values {
			entryBytes := int64(len(key) + len(value) + 4)
			if used+entryBytes > manualCaptureHeaderLimit {
				truncated = true
				continue
			}
			result[key] = append(result[key], value)
			used += entryBytes
		}
	}
	return result, truncated
}

func readBoundedResponseBody(body io.Reader) ([]byte, bool, error) {
	limited := io.LimitReader(body, manualCaptureBodyLimit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) <= manualCaptureBodyLimit {
		return data, false, nil
	}
	return data[:manualCaptureBodyLimit], true, nil
}
