package handler

import (
	"net/url"
	"strings"
)

// sanitizeErrorForLog 保留诊断文本，同时移除常见密钥形态。
func sanitizeErrorForLog(err error) string {
	if err == nil {
		return ""
	}
	message := stripURLSecrets(err.Error())
	for _, marker := range []string{"x-api-key", "api_key", "apikey", "token", "authorization", "bearer"} {
		message = redactAfterMarker(message, marker)
	}
	return message
}

// redactAfterMarker 隐藏敏感标记后紧跟的值，避免日志泄漏鉴权材料。
func redactAfterMarker(value string, marker string) string {
	lower := strings.ToLower(value)
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return value
	}

	start := idx + len(marker)
	for start < len(value) && (value[start] == ' ' || value[start] == ':' || value[start] == '=' || value[start] == '"' || value[start] == '\'') {
		start++
	}
	end := start
	for end < len(value) && value[end] != ' ' && value[end] != ',' && value[end] != '&' && value[end] != ';' && value[end] != '"' && value[end] != '\'' {
		end++
	}
	if start == end {
		return value
	}
	return value[:start] + "[REDACTED]" + value[end:]
}

// sanitizeURLForLog 在写诊断日志前移除查询串和 fragment。
func sanitizeURLForLog(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return stripURLSecrets(value)
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// stripURLSecrets 是来源头里出现畸形 URL 时的防御性兜底。
func stripURLSecrets(value string) string {
	if idx := strings.IndexAny(value, "?#"); idx >= 0 {
		return value[:idx]
	}
	return value
}
