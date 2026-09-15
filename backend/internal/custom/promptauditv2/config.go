package promptauditv2

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

const (
	// promptPlaceholder is deliberately exact so templates cannot silently omit or duplicate user input.
	promptPlaceholder = "{{user_input}}"
	// maxWorkerCount bounds model-call concurrency created by one process.
	maxWorkerCount = 64
	// maxQueueCapacity bounds retained request bodies waiting for synchronous audit.
	maxQueueCapacity = 10000
	// maxRetentionDays prevents an accidental effectively-unbounded hit-log policy.
	maxRetentionDays = 3650
	// maxRuleWindowMinutes caps one rolling counter at seven days.
	maxRuleWindowMinutes = 10080
	// maxRestrictionMinutes caps a warning restriction at thirty days.
	maxRestrictionMinutes = 43200
	// maxTriggerCount prevents impractical counters and integer abuse.
	maxTriggerCount = 10000
	// minEndpointTimeoutMS rejects timeouts too short to be operationally meaningful.
	minEndpointTimeoutMS = 100
	// maxEndpointTimeoutMS caps how long a gateway request may wait on one endpoint.
	maxEndpointTimeoutMS = 120000
)

var stableIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// SupportedProtocols returns the complete chat-protocol catalog accepted by extraction.
func SupportedProtocols() []ProtocolOption {
	return []ProtocolOption{
		{Value: ProtocolOpenAIChat, Label: "OpenAI Chat Completions"},
		{Value: ProtocolOpenAIResponses, Label: "OpenAI Responses"},
		{Value: ProtocolAnthropicMessages, Label: "Anthropic Messages"},
		{Value: ProtocolGemini, Label: "Gemini"},
	}
}

// PrepareConfig validates a full replacement request, preserves omitted existing
// credentials, and returns a deterministic snapshot.
func PrepareConfig(request UpdateConfigRequest, existing *Config) (Config, error) {
	if request.ExpectedConfigVersion <= 0 {
		return Config{}, invalidConfig("expected_config_version must be positive")
	}
	if (request.Enabled || request.PromptTemplate != "") && strings.Count(request.PromptTemplate, promptPlaceholder) != 1 {
		return Config{}, invalidConfig("prompt_template must contain exactly one {{user_input}}")
	}
	if (request.Enabled || request.WorkerCount != 0) && (request.WorkerCount < 1 || request.WorkerCount > maxWorkerCount) {
		return Config{}, invalidConfig("worker_count must be between 1 and 64")
	}
	if (request.Enabled || request.QueueCapacity != 0) && (request.QueueCapacity < 1 || request.QueueCapacity > maxQueueCapacity) {
		return Config{}, invalidConfig("queue_capacity must be between 1 and 10000")
	}
	if (request.Enabled || request.LogRetentionDays != 0) && (request.LogRetentionDays < 1 || request.LogRetentionDays > maxRetentionDays) {
		return Config{}, invalidConfig("log_retention_days must be between 1 and 3650")
	}

	protocols, err := validateProtocols(request.EnabledProtocols)
	if err != nil {
		return Config{}, err
	}
	endpoints, err := prepareEndpoints(request.Endpoints, existing, request.Enabled)
	if err != nil {
		return Config{}, err
	}
	rule, err := prepareRule(request.Rule)
	if err != nil {
		return Config{}, err
	}
	if request.LogRetentionDays > 0 && request.LogRetentionDays*24*60 < rule.WindowMinutes {
		return Config{}, invalidConfig("log retention is shorter than rule rolling window")
	}
	if request.Enabled {
		if len(protocols) == 0 {
			return Config{}, invalidConfig("enabled configuration requires at least one protocol")
		}
		if countEnabledEndpoints(endpoints) == 0 {
			return Config{}, invalidConfig("enabled configuration requires at least one enabled endpoint")
		}
	}

	return Config{
		Enabled:          request.Enabled,
		PromptTemplate:   request.PromptTemplate,
		WorkerCount:      request.WorkerCount,
		QueueCapacity:    request.QueueCapacity,
		EnabledProtocols: protocols,
		LogRetentionDays: request.LogRetentionDays,
		ConfigVersion:    request.ExpectedConfigVersion + 1,
		Endpoints:        endpoints,
		Rule:             rule,
	}, nil
}

// BuildRuntimeConfig validates enabled endpoint keys and builds the immutable
// runtime snapshot used by one worker-pool generation.
func BuildRuntimeConfig(config Config) (*RuntimeConfig, error) {
	validatedProtocols, err := validateProtocols(config.EnabledProtocols)
	if err != nil {
		return nil, err
	}
	protocols := make(map[string]struct{}, len(config.EnabledProtocols))
	for _, protocol := range validatedProtocols {
		protocols[protocol] = struct{}{}
	}
	active := &RuntimeConfig{
		Enabled:          config.Enabled,
		PromptTemplate:   config.PromptTemplate,
		WorkerCount:      config.WorkerCount,
		QueueCapacity:    config.QueueCapacity,
		EnabledProtocols: protocols,
		LogRetentionDays: config.LogRetentionDays,
		ConfigVersion:    config.ConfigVersion,
		Rule:             config.Rule,
	}
	if !config.Enabled {
		return active, nil
	}
	if strings.Count(config.PromptTemplate, promptPlaceholder) != 1 ||
		config.WorkerCount < 1 || config.WorkerCount > maxWorkerCount ||
		config.QueueCapacity < 1 || config.QueueCapacity > maxQueueCapacity ||
		config.LogRetentionDays < 1 || config.LogRetentionDays > maxRetentionDays ||
		len(protocols) == 0 || countEnabledEndpoints(config.Endpoints) == 0 {
		return nil, invalidConfig("enabled runtime configuration is incomplete")
	}
	validatedEndpoints, err := prepareEndpoints(config.Endpoints, nil, true)
	if err != nil {
		return nil, err
	}
	validatedRule, err := prepareRule(config.Rule)
	if err != nil {
		return nil, err
	}
	if config.LogRetentionDays*24*60 < validatedRule.WindowMinutes {
		return nil, invalidConfig("log retention is shorter than rule rolling window")
	}
	active.Rule = validatedRule
	active.Endpoints, err = buildActiveEndpoints(validatedEndpoints)
	if err != nil {
		return nil, err
	}
	return active, nil
}

// PublicConfig removes write-only endpoint credentials before an HTTP response.
func PublicConfig(config Config) Config {
	config.Protocols = SupportedProtocols()
	// Start from non-nil slices so valid empty collections keep the API contract as [] instead of null.
	config.EnabledProtocols = append([]string{}, config.EnabledProtocols...)
	config.Endpoints = append([]EndpointConfig{}, config.Endpoints...)
	for index := range config.Endpoints {
		config.Endpoints[index].APIKey = ""
	}
	return config
}

func validateProtocols(values []string) ([]string, error) {
	allowed := make(map[string]struct{}, len(SupportedProtocols()))
	for _, option := range SupportedProtocols() {
		allowed[option.Value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(values))
	protocols := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if _, ok := allowed[value]; !ok {
			return nil, invalidConfig(fmt.Sprintf("unsupported protocol %q", value))
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, invalidConfig(fmt.Sprintf("duplicate protocol %q", value))
		}
		seen[value] = struct{}{}
		protocols = append(protocols, value)
	}
	return protocols, nil
}

func prepareEndpoints(values []EndpointConfig, existing *Config, requireEnabledSecrets bool) ([]EndpointConfig, error) {
	existingKeys := map[string]string{}
	if existing != nil {
		for _, endpoint := range existing.Endpoints {
			existingKeys[endpoint.ID] = endpoint.APIKey
		}
	}
	seen := make(map[string]struct{}, len(values))
	endpoints := make([]EndpointConfig, 0, len(values))
	for index, raw := range values {
		endpoint := raw
		endpoint.ID = strings.TrimSpace(endpoint.ID)
		endpoint.Name = strings.TrimSpace(endpoint.Name)
		endpoint.BaseURL = strings.TrimSpace(endpoint.BaseURL)
		endpoint.Model = strings.TrimSpace(endpoint.Model)
		endpoint.Order = index
		if !stableIDPattern.MatchString(endpoint.ID) {
			return nil, invalidConfig("endpoint id must be a stable 1-64 character identifier")
		}
		if _, duplicate := seen[endpoint.ID]; duplicate {
			return nil, invalidConfig(fmt.Sprintf("duplicate endpoint id %q", endpoint.ID))
		}
		seen[endpoint.ID] = struct{}{}
		if endpoint.Name == "" || endpoint.Model == "" {
			return nil, invalidConfig(fmt.Sprintf("endpoint %q requires name and model", endpoint.ID))
		}
		if _, err := normalizeChatCompletionsURL(endpoint.BaseURL); err != nil {
			return nil, invalidConfig(fmt.Sprintf("endpoint %q base_url is invalid", endpoint.ID))
		}
		if endpoint.Priority < -100000 || endpoint.Priority > 100000 {
			return nil, invalidConfig(fmt.Sprintf("endpoint %q priority is out of range", endpoint.ID))
		}
		if endpoint.TimeoutMS < minEndpointTimeoutMS || endpoint.TimeoutMS > maxEndpointTimeoutMS {
			return nil, invalidConfig(fmt.Sprintf("endpoint %q timeout_ms is out of range", endpoint.ID))
		}
		if strings.TrimSpace(endpoint.APIKey) == "" {
			endpoint.APIKey = existingKeys[endpoint.ID]
		}
		endpoint.APIKey = strings.TrimSpace(endpoint.APIKey)
		endpoint.HasAPIKey = endpoint.APIKey != ""
		if requireEnabledSecrets && endpoint.Enabled && !endpoint.HasAPIKey {
			return nil, invalidConfig(fmt.Sprintf("enabled endpoint %q requires an api key", endpoint.ID))
		}
		endpoints = append(endpoints, endpoint)
	}
	return endpoints, nil
}

func prepareRule(raw RuleConfig) (RuleConfig, error) {
	rule := raw
	rule.Action = strings.TrimSpace(rule.Action)
	if rule.ConfidenceThreshold < 0 || rule.ConfidenceThreshold > 1 {
		return RuleConfig{}, invalidConfig("rule confidence_threshold is out of range")
	}
	if rule.WindowMinutes < 1 || rule.WindowMinutes > maxRuleWindowMinutes {
		return RuleConfig{}, invalidConfig("rule window_minutes is out of range")
	}
	if rule.TriggerCount < 1 || rule.TriggerCount > maxTriggerCount {
		return RuleConfig{}, invalidConfig("rule trigger_count is out of range")
	}
	switch rule.Action {
	case ActionBan:
		if rule.RestrictionMinutes != nil {
			return RuleConfig{}, invalidConfig("ban rule cannot set restriction_minutes")
		}
	case ActionWarning:
		if rule.RestrictionMinutes == nil || *rule.RestrictionMinutes < 1 || *rule.RestrictionMinutes > maxRestrictionMinutes {
			return RuleConfig{}, invalidConfig("warning rule requires valid restriction_minutes")
		}
	default:
		return RuleConfig{}, invalidConfig("rule action is invalid")
	}
	return rule, nil
}

func normalizeChatCompletionsURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("base url must be an absolute HTTP URL without query or fragment")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("base url scheme must be http or https")
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		parsed.Path = path
	case strings.HasSuffix(path, "/v1"):
		parsed.Path = path + "/chat/completions"
	default:
		parsed.Path = path + "/v1/chat/completions"
	}
	return parsed.String(), nil
}

func countEnabledEndpoints(values []EndpointConfig) int {
	count := 0
	for _, endpoint := range values {
		if endpoint.Enabled {
			count++
		}
	}
	return count
}

func invalidConfig(detail string) error {
	return ErrInvalidConfig.WithMetadata(map[string]string{"detail": detail})
}
