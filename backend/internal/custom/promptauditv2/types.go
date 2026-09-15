package promptauditv2

import "time"

const (
	// ProtocolOpenAIChat identifies OpenAI-compatible chat-completions request bodies.
	ProtocolOpenAIChat = "openai_chat_completions"
	// ProtocolOpenAIResponses identifies OpenAI Responses request bodies and WebSocket turns.
	ProtocolOpenAIResponses = "openai_responses"
	// ProtocolAnthropicMessages identifies Anthropic Messages request bodies.
	ProtocolAnthropicMessages = "anthropic_messages"
	// ProtocolGemini identifies Gemini generate-content request bodies.
	ProtocolGemini = "gemini"
	// ActionBan permanently disables the user when the rule threshold is reached.
	ActionBan = "ban"
	// ActionWarning blocks gateway access until a calculated expiry time.
	ActionWarning = "warning"
)

// ProtocolOption is returned by the backend so the page never invents protocol identifiers.
type ProtocolOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// EndpointConfig describes one OpenAI-compatible audit model service.
// APIKey is write-only at the HTTP boundary; PublicConfig clears it before responses.
type EndpointConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key,omitempty"`
	HasAPIKey bool   `json:"has_api_key"`
	Model     string `json:"model"`
	Priority  int    `json:"priority"`
	TimeoutMS int    `json:"timeout_ms"`
	Enabled   bool   `json:"enabled"`
	Order     int    `json:"order"`
}

// RuleConfig defines the single confidence rule and its rolling threshold action.
type RuleConfig struct {
	ConfidenceThreshold float64 `json:"confidence_threshold"`
	WindowMinutes       int     `json:"window_minutes"`
	TriggerCount        int     `json:"trigger_count"`
	Action              string  `json:"action"`
	RestrictionMinutes  *int    `json:"restriction_minutes,omitempty"`
}

// Config is the persisted and administrator-visible module configuration.
type Config struct {
	Enabled          bool             `json:"enabled"`
	PromptTemplate   string           `json:"prompt_template"`
	WorkerCount      int              `json:"worker_count"`
	QueueCapacity    int              `json:"queue_capacity"`
	EnabledProtocols []string         `json:"enabled_protocols"`
	LogRetentionDays int              `json:"log_retention_days"`
	ConfigVersion    int64            `json:"config_version"`
	UpdatedAt        time.Time        `json:"updated_at"`
	UpdatedBy        *int64           `json:"updated_by,omitempty"`
	Endpoints        []EndpointConfig `json:"endpoints"`
	Rule             RuleConfig       `json:"rule"`
	Protocols        []ProtocolOption `json:"protocols,omitempty"`
}

// UpdateConfigRequest replaces the complete configuration using optimistic locking.
type UpdateConfigRequest struct {
	ExpectedConfigVersion int64            `json:"expected_config_version"`
	Enabled               bool             `json:"enabled"`
	PromptTemplate        string           `json:"prompt_template"`
	WorkerCount           int              `json:"worker_count"`
	QueueCapacity         int              `json:"queue_capacity"`
	EnabledProtocols      []string         `json:"enabled_protocols"`
	LogRetentionDays      int              `json:"log_retention_days"`
	Endpoints             []EndpointConfig `json:"endpoints"`
	Rule                  RuleConfig       `json:"rule"`
}

// ActiveEndpoint contains the credentials used only by audit workers.
type ActiveEndpoint struct {
	ID       string
	Name     string
	URL      string
	APIKey   string
	Model    string
	Priority int
	Timeout  time.Duration
	Order    int
}

// RuntimeConfig is an immutable snapshot shared by one worker-pool generation.
type RuntimeConfig struct {
	Enabled          bool
	PromptTemplate   string
	WorkerCount      int
	QueueCapacity    int
	EnabledProtocols map[string]struct{}
	LogRetentionDays int
	ConfigVersion    int64
	Endpoints        []ActiveEndpoint
	Rule             RuleConfig
}

// AuditResult is the strict result returned by one model endpoint.
type AuditResult struct {
	Confidence   float64
	Reason       string
	EndpointID   string
	EndpointName string
	AuditModel   string
	LatencyMS    int
}

// Restriction is the database-authoritative gateway restriction for one user.
type Restriction struct {
	UserID       int64      `json:"user_id"`
	EventID      *int64     `json:"event_id,omitempty"`
	Action       string     `json:"action"`
	StartedAt    time.Time  `json:"started_at"`
	BlockedUntil *time.Time `json:"blocked_until,omitempty"`
}

// HitInput contains the immutable snapshots required for one transactional hit.
type HitInput struct {
	RequestID    string
	UserID       int64
	Username     string
	UserEmail    string
	APIKeyID     int64
	APIKeyName   string
	Protocol     string
	RequestModel string
	Message      string
	Result       AuditResult
	Rule         RuleConfig
}

// HitOutcome reports the committed rolling count and enforcement decision.
type HitOutcome struct {
	EventID          int64
	WindowHitCount   int
	ThresholdReached bool
	Action           string
	BlockedUntil     *time.Time
}

// Event is one rule-hit log. Message is populated only by the detail endpoint.
type Event struct {
	ID                     int64     `json:"id"`
	RequestID              string    `json:"request_id"`
	UserID                 int64     `json:"user_id"`
	Username               string    `json:"username"`
	UserEmail              string    `json:"user_email"`
	APIKeyID               int64     `json:"api_key_id"`
	APIKeyName             string    `json:"api_key_name"`
	Protocol               string    `json:"protocol"`
	RequestModel           string    `json:"request_model"`
	EndpointID             string    `json:"endpoint_id"`
	EndpointName           string    `json:"endpoint_name"`
	AuditModel             string    `json:"audit_model"`
	Confidence             float64   `json:"confidence"`
	Reason                 string    `json:"reason"`
	LatencyMS              int       `json:"latency_ms"`
	RuleThreshold          float64   `json:"rule_threshold"`
	RuleWindowMinutes      int       `json:"rule_window_minutes"`
	RuleTriggerCount       int       `json:"rule_trigger_count"`
	RuleAction             string    `json:"rule_action"`
	RuleRestrictionMinutes *int      `json:"rule_restriction_minutes,omitempty"`
	WindowHitCount         int       `json:"window_hit_count"`
	ThresholdReached       bool      `json:"threshold_reached"`
	FinalAction            string    `json:"final_action"`
	ActionResult           string    `json:"action_result"`
	MessageSHA256          string    `json:"message_sha256"`
	MessageChars           int       `json:"message_chars"`
	Message                string    `json:"message,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	MessageCiphertext      string    `json:"-"`
}

// EventFilter is the allowlisted query surface for hit-only event pagination.
type EventFilter struct {
	StartAt       *time.Time
	EndAt         *time.Time
	UserID        *int64
	Action        string
	Protocol      string
	MinConfidence *float64
	MaxConfidence *float64
}

// EventPage is the stable paginated event response.
type EventPage struct {
	Items    []Event `json:"items"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"page_size"`
}

// EmailJob is the persisted delivery lease consumed by the background worker.
type EmailJob struct {
	ID             int64
	EventID        int64
	RecipientEmail string
	Attempts       int
	Event          Event
}

// EndpointRuntime summarizes the last probe or model-call health without exposing URLs or keys.
type EndpointRuntime struct {
	OK        bool      `json:"ok"`
	Status    string    `json:"status"`
	LatencyMS int       `json:"latency_ms"`
	CheckedAt time.Time `json:"checked_at"`
}

// HealthSnapshot is the shared Redis state produced by one complete probe round.
// AvailableEndpoints is already ordered by endpoint priority for gateway use.
type HealthSnapshot struct {
	ConfigVersion      int64                      `json:"config_version"`
	CheckedAt          time.Time                  `json:"checked_at"`
	Endpoints          map[string]EndpointRuntime `json:"endpoints"`
	AvailableEndpoints []string                   `json:"available_endpoints"`
}

// RuntimeSnapshot exposes bounded-queue and delivery state for administrators.
type RuntimeSnapshot struct {
	Status              string                     `json:"status"`
	ConfigVersion       int64                      `json:"config_version"`
	HealthConfigVersion int64                      `json:"health_config_version"`
	HealthCheckedAt     *time.Time                 `json:"health_checked_at,omitempty"`
	HealthErrorCode     string                     `json:"health_error_code,omitempty"`
	WorkerCount         int                        `json:"worker_count"`
	QueueCapacity       int                        `json:"queue_capacity"`
	Queued              int                        `json:"queued"`
	Processing          int64                      `json:"processing"`
	Failovers           int64                      `json:"failovers"`
	Hits                int64                      `json:"hits"`
	Rejected            int64                      `json:"rejected"`
	Unavailable         int64                      `json:"unavailable"`
	EmailPending        int64                      `json:"email_pending"`
	EmailFailed         int64                      `json:"email_failed"`
	Endpoints           map[string]EndpointRuntime `json:"endpoints"`
	LastErrorCode       string                     `json:"last_error_code,omitempty"`
	LastErrorAt         *time.Time                 `json:"last_error_at,omitempty"`
}

// ProbeRequest tests one draft endpoint without persisting its credentials.
type ProbeRequest struct {
	Endpoint EndpointConfig `json:"endpoint"`
}

// ProbeResult reports strict protocol compatibility without returning the endpoint URL or key.
type ProbeResult struct {
	OK         bool      `json:"ok"`
	Status     string    `json:"status"`
	HTTPStatus int       `json:"http_status"`
	LatencyMS  int       `json:"latency_ms"`
	ErrorCode  string    `json:"error_code,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// PromptTestRequest carries an unsaved prompt and endpoint draft for one
// side-effect-free administrator test.
type PromptTestRequest struct {
	PromptTemplate string           `json:"prompt_template"`
	UserInput      string           `json:"user_input"`
	Endpoints      []EndpointConfig `json:"endpoints"`
}

// PromptTestResult exposes only the successful model decision and non-secret
// service metadata required by the administrator result panel.
type PromptTestResult struct {
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason"`
	EndpointName string  `json:"endpoint_name"`
	AuditModel   string  `json:"audit_model"`
	LatencyMS    int     `json:"latency_ms"`
}
