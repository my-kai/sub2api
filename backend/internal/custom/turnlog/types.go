package turnlog

import "time"

const (
	// DefaultRetentionDays is the initial administrator-configured retention period in days.
	DefaultRetentionDays = 30
	// MinRetentionDays is the smallest retention period accepted by the API.
	MinRetentionDays = 1
	// MaxRetentionDays is the largest retention period accepted by the API.
	MaxRetentionDays = 3650
	// MaxHeadersBytes bounds serialized response headers retained per event.
	MaxHeadersBytes int64 = 256 << 10
	// MaxBodyBytes bounds response body bytes retained per event.
	MaxBodyBytes int64 = 2 << 20
	// queueCapacity prevents upstream request bursts from growing process memory without bound.
	queueCapacity = 1024
)

// TurnLog is the administrator-visible snapshot of one matching upstream response.
type TurnLog struct {
	ID               int64               `json:"id"`
	AccountID        int64               `json:"account_id"`
	AccountName      string              `json:"account_name"`
	StatusCode       int                 `json:"status_code"`
	ResponseHeaders  map[string][]string `json:"response_headers"`
	ResponseBody     string              `json:"response_body"`
	HeadersTruncated bool                `json:"headers_truncated"`
	BodyTruncated    bool                `json:"body_truncated"`
	BodyComplete     bool                `json:"body_complete"`
	CreatedAt        time.Time           `json:"created_at"`
}

// TurnLogConfig controls how long captured diagnostic rows remain queryable.
type TurnLogConfig struct {
	RetentionDays int       `json:"retention_days"`
	UpdatedAt     time.Time `json:"updated_at"`
	UpdatedBy     *int64    `json:"updated_by,omitempty"`
}

// TurnLogFilter contains the supported administrator list filters.
type TurnLogFilter struct {
	AccountID  *int64
	StatusCode *int
	Page       int
	PageSize   int
}

// TurnLogPage is the stable paginated list response.
type TurnLogPage struct {
	Items    []TurnLog `json:"items"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}

// queuedEvent is internal worker input and is deliberately not exposed by the API.
type queuedEvent struct {
	AccountID        int64
	StatusCode       int
	Headers          map[string][]string
	ResponseBody     string
	HeadersTruncated bool
	BodyTruncated    bool
	BodyComplete     bool
}
