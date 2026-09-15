package promptauditv2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type auditMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type geminiContent struct {
	Role  string `json:"role"`
	Parts []struct {
		Text string `json:"text"`
	} `json:"parts"`
}

// ExtractLatestUserMessage returns only the latest user-authored text in the
// current request. It never substitutes the full body or historical messages.
func ExtractLatestUserMessage(protocol string, body []byte) (string, bool, error) {
	switch protocol {
	case ProtocolOpenAIChat, ProtocolAnthropicMessages:
		var payload struct {
			Messages []auditMessage `json:"messages"`
		}
		if err := decodeRequestObject(body, &payload); err != nil {
			return "", false, err
		}
		return latestMessageText(payload.Messages)
	case ProtocolOpenAIResponses:
		return extractResponsesMessage(body)
	case ProtocolGemini:
		var payload struct {
			Contents []geminiContent `json:"contents"`
		}
		if err := decodeRequestObject(body, &payload); err != nil {
			return "", false, err
		}
		for index := len(payload.Contents) - 1; index >= 0; index-- {
			if strings.EqualFold(strings.TrimSpace(payload.Contents[index].Role), "user") {
				parts := make([]string, 0, len(payload.Contents[index].Parts))
				for _, part := range payload.Contents[index].Parts {
					if text := strings.TrimSpace(part.Text); text != "" {
						parts = append(parts, text)
					}
				}
				text := strings.Join(parts, "\n")
				return text, text != "", nil
			}
		}
		return "", false, nil
	default:
		return "", false, fmt.Errorf("unsupported prompt audit v2 protocol %q", protocol)
	}
}

func extractResponsesMessage(body []byte) (string, bool, error) {
	var payload struct {
		Input   json.RawMessage `json:"input"`
		Item    *auditMessage   `json:"item"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := decodeRequestObject(body, &payload); err != nil {
		return "", false, err
	}
	if payload.Item != nil && strings.EqualFold(strings.TrimSpace(payload.Item.Role), "user") {
		text, err := messageContentText(payload.Item.Content)
		return text, text != "", err
	}
	if strings.EqualFold(strings.TrimSpace(payload.Role), "user") {
		text, err := messageContentText(payload.Content)
		return text, text != "", err
	}
	trimmed := bytes.TrimSpace(payload.Input)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", false, nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return "", false, fmt.Errorf("decode responses input string: %w", err)
		}
		text = strings.TrimSpace(text)
		return text, text != "", nil
	}
	var messages []auditMessage
	if err := json.Unmarshal(trimmed, &messages); err != nil {
		return "", false, fmt.Errorf("decode responses input messages: %w", err)
	}
	return latestMessageText(messages)
}

func latestMessageText(messages []auditMessage) (string, bool, error) {
	for index := len(messages) - 1; index >= 0; index-- {
		if !strings.EqualFold(strings.TrimSpace(messages[index].Role), "user") {
			continue
		}
		text, err := messageContentText(messages[index].Content)
		if err != nil {
			return "", false, err
		}
		return text, text != "", nil
	}
	return "", false, nil
}

func messageContentText(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", nil
	}
	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return "", fmt.Errorf("decode user message text: %w", err)
		}
		return strings.TrimSpace(text), nil
	}
	var parts []struct {
		Type string          `json:"type"`
		Text json.RawMessage `json:"text"`
	}
	if err := json.Unmarshal(trimmed, &parts); err != nil {
		return "", fmt.Errorf("decode user message parts: %w", err)
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(bytes.TrimSpace(part.Text)) == 0 {
			continue
		}
		var text string
		if err := json.Unmarshal(part.Text, &text); err != nil {
			return "", fmt.Errorf("decode user message part: %w", err)
		}
		if text = strings.TrimSpace(text); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func decodeRequestObject(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode prompt audit v2 request: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("decode prompt audit v2 request: trailing JSON value")
	}
	return nil
}
