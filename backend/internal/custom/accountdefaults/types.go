package accountdefaults

// ProxyMode controls how the create-account form pre-fills the egress proxy.
type ProxyMode string

const (
	// ProxyModeFixed pre-fills every ID in ProxyFixedIDs (as long as still available).
	ProxyModeFixed ProxyMode = "fixed"
	// ProxyModeRandom pre-fills exactly one random available proxy at form open.
	ProxyModeRandom ProxyMode = "random"
)

// Config is the administrator-configurable default state for the create-account form.
type Config struct {
	// ModelsByPlatform maps a platform id (openai/anthropic/gemini/...) to the model
	// whitelist pre-selected on the form. Missing or empty list means "no default
	// restriction" for that platform (account defaults to supporting all models).
	ModelsByPlatform map[string][]string `json:"models_by_platform"`
	// ProxyMode is "fixed" or "random".
	ProxyMode ProxyMode `json:"proxy_mode"`
	// ProxyFixedIDs are the pre-selected proxy IDs when ProxyMode == fixed.
	ProxyFixedIDs []int64 `json:"proxy_fixed_ids"`
	// AllowLocalEgress pre-fills the "direct connection" exit when ProxyMode == fixed.
	AllowLocalEgress bool `json:"allow_local_egress"`
}

// ResolvedDefaults is what the create-account form receives. Random proxy selection
// is resolved server-side (one draw at form open) so the form shows a concrete proxy.
type ResolvedDefaults struct {
	ModelsByPlatform map[string][]string `json:"models_by_platform"`
	ProxyMode        ProxyMode           `json:"proxy_mode"`
	// EgressProxyIDs is the concrete pre-selected proxy list (0 length if none).
	EgressProxyIDs []int64 `json:"egress_proxy_ids"`
	// EgressIncludeLocal mirrors the direct-connection toggle.
	EgressIncludeLocal bool `json:"egress_include_local"`
}
