package promptauditv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// maxAuditResponseBytes bounds untrusted model responses retained in memory.
	maxAuditResponseBytes = 1024 * 1024
)

// EndpointCallError classifies one endpoint failure for failover and probes.
type EndpointCallError struct {
	Code       string
	HTTPStatus int
	Cause      error
}

// Error returns a credential-free diagnostic string.
func (e *EndpointCallError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return e.Code
	}
	return fmt.Sprintf("%s: %v", e.Code, e.Cause)
}

// Unwrap exposes the transport or parsing cause without changing the stable code.
func (e *EndpointCallError) Unwrap() error { return e.Cause }

// OpenAIClient performs stateless Chat Completions audits.
type OpenAIClient struct {
	client *http.Client
}

// NewOpenAIClient creates a client whose timeout is controlled per endpoint.
func NewOpenAIClient() *OpenAIClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 32
	return &OpenAIClient{client: &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("prompt audit v2 endpoint redirects are disabled")
		},
	}}
}

// Audit sends exactly one user message containing the rendered audit template
// and strictly parses the required confidence/reason object.
func (c *OpenAIClient) Audit(ctx context.Context, endpoint ActiveEndpoint, template, userInput string) (AuditResult, error) {
	if c == nil || c.client == nil {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeUnavailable, Cause: errors.New("http client is not initialized")}
	}
	callCtx, cancel := context.WithTimeout(ctx, endpoint.Timeout)
	defer cancel()
	payload := struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{Model: endpoint.Model}
	payload.Messages = append(payload.Messages, struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{Role: "user", Content: strings.Replace(template, promptPlaceholder, userInput, 1)})
	body, err := json.Marshal(payload)
	if err != nil {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeUnavailable, Cause: err}
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeUnavailable, Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+endpoint.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sub2api-prompt-audit-v2")
	startedAt := time.Now()
	resp, err := c.client.Do(req)
	latencyMS := int(time.Since(startedAt).Milliseconds())
	if err != nil {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeUnavailable, Cause: err}
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxAuditResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeUnavailable, HTTPStatus: resp.StatusCode, Cause: err}
	}
	if len(responseBody) > maxAuditResponseBytes {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeInvalidResponse, HTTPStatus: resp.StatusCode, Cause: errors.New("response exceeds size limit")}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeUnavailable, HTTPStatus: resp.StatusCode, Cause: fmt.Errorf("endpoint returned HTTP %d", resp.StatusCode)}
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil || len(envelope.Choices) != 1 {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeInvalidResponse, HTTPStatus: resp.StatusCode, Cause: errors.New("chat completion response must contain one choice")}
	}
	var content string
	if err := json.Unmarshal(envelope.Choices[0].Message.Content, &content); err != nil {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeInvalidResponse, HTTPStatus: resp.StatusCode, Cause: errors.New("choice content must be a JSON string")}
	}
	parsed, err := parseStrictAuditResult(content)
	if err != nil {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeInvalidResponse, HTTPStatus: resp.StatusCode, Cause: err}
	}
	parsed.EndpointID = endpoint.ID
	parsed.EndpointName = endpoint.Name
	parsed.AuditModel = endpoint.Model
	parsed.LatencyMS = latencyMS
	return parsed, nil
}

func parseStrictAuditResult(content string) (AuditResult, error) {
	var value struct {
		Confidence *float64 `json:"confidence"`
		Reason     *string  `json:"reason"`
	}
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return AuditResult{}, fmt.Errorf("decode strict audit result: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return AuditResult{}, errors.New("strict audit result contains trailing data")
	}
	if value.Confidence == nil || value.Reason == nil {
		return AuditResult{}, errors.New("strict audit result requires confidence and reason")
	}
	if *value.Confidence < 0 || *value.Confidence > 1 {
		return AuditResult{}, errors.New("strict audit result confidence is out of range")
	}
	return AuditResult{Confidence: *value.Confidence, Reason: *value.Reason}, nil
}
