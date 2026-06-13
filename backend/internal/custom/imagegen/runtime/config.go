package runtime

import (
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultImageClientTimeout = 10 * time.Minute
)

// Config 保存 custom 生图模块启动期配置。
//
// 该结构只从环境变量读取，避免把二开字段写入主仓全局 Config 结构，降低后续合并
// 上游配置文件时的冲突概率。未配置上游地址或鉴权密钥时，模块仍会启动，页面会显示空模型，
// 创建任务后的 worker 会把任务标记为配置缺失失败。
type Config struct {
	TablePrefix        string
	ChatGPT2APIBaseURL *url.URL
	ChatGPT2APIAuthKey string
	HTTPTimeout        time.Duration
}

// LoadConfigFromEnv 读取 custom 生图模块所需的环境变量。
//
// 支持变量：
//   - CUSTOM_IMAGEGEN_TABLE_PREFIX：custom 表名前缀，默认空。
//   - CHATGPT2API_BASE_URL：OpenAI 兼容生图上游地址，未配置时禁用上游调用。
//   - CHATGPT2API_AUTH_KEY：传给 chatgpt2api 的 auth-key。
//   - IMAGE_CLIENT_TIMEOUT：上游请求超时，默认 10m。
func LoadConfigFromEnv() (Config, error) {
	baseURL, err := parseOptionalHTTPURL(os.Getenv("CHATGPT2API_BASE_URL"))
	if err != nil {
		return Config{}, err
	}
	timeout, err := parseDurationWithDefault(os.Getenv("IMAGE_CLIENT_TIMEOUT"), defaultImageClientTimeout)
	if err != nil {
		return Config{}, err
	}
	return Config{
		TablePrefix:        strings.TrimSpace(os.Getenv("CUSTOM_IMAGEGEN_TABLE_PREFIX")),
		ChatGPT2APIBaseURL: baseURL,
		ChatGPT2APIAuthKey: strings.TrimSpace(os.Getenv("CHATGPT2API_AUTH_KEY")),
		HTTPTimeout:        timeout,
	}, nil
}

// ValidateTablePrefix 限制表名前缀只能由字母、数字和下划线组成。
//
// Store 会把表名前缀拼进 SQL 标识符，必须在启动装配阶段先做白名单校验。
func ValidateTablePrefix(prefix string) error {
	return validateIdentifierPart(strings.TrimSpace(prefix), true)
}

func parseOptionalHTTPURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrInvalidConfig("CHATGPT2API_BASE_URL must use http or https scheme")
	}
	if parsed.Host == "" {
		return nil, ErrInvalidConfig("CHATGPT2API_BASE_URL must include host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed, nil
}

func parseDurationWithDefault(raw string, fallback time.Duration) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, ErrInvalidConfig("IMAGE_CLIENT_TIMEOUT must be positive")
	}
	return parsed, nil
}

// ErrInvalidConfig 标识 custom 生图启动配置错误。
type ErrInvalidConfig string

func (e ErrInvalidConfig) Error() string {
	return string(e)
}

func validateIdentifierPart(value string, allowEmpty bool) error {
	if value == "" {
		if allowEmpty {
			return nil
		}
		return ErrInvalidConfig("identifier is required")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return ErrInvalidConfig("identifier may only contain letters, digits or underscores")
	}
	return nil
}
