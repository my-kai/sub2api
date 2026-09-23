package service

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// explicitSessionIDContextKey carries only a client-provided session identity.
// It is deliberately separate from the broader account-sticky hash so content
// fallbacks can never create a new egress-sticky binding.
type explicitSessionIDContextKey struct{}

var explicitSessionIDKey = explicitSessionIDContextKey{}

// WithExplicitSessionID attaches a sanitized client session identifier to the
// request context. Empty or invalid values remove the egress-sticky signal.
func WithExplicitSessionID(ctx context.Context, sessionID string) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, explicitSessionIDKey, sanitizeSessionID(sessionID))
}

// ExplicitSessionIDFromContext returns the explicit client session identifier,
// excluding all content-derived and other account-sticky fallbacks.
func ExplicitSessionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	sessionID, _ := ctx.Value(explicitSessionIDKey).(string)
	return sanitizeSessionID(sessionID)
}

// AttachExplicitSessionIDToGin stores the explicit session signal on the Gin
// request context while preserving all existing request-scoped values.
func AttachExplicitSessionIDToGin(c *gin.Context, sessionID string) {
	if c == nil || c.Request == nil {
		return
	}
	c.Request = c.Request.WithContext(WithExplicitSessionID(c.Request.Context(), sessionID))
}

// ExplicitSessionIDFromParsedRequest extracts only the metadata session ID that
// is explicitly supplied by a generic gateway client. Content summaries remain
// available to GenerateSessionHash for the existing account sticky path only.
func ExplicitSessionIDFromParsedRequest(parsed *ParsedRequest) string {
	if parsed == nil || strings.TrimSpace(parsed.MetadataUserID) == "" {
		return ""
	}
	uid := ParseMetadataUserID(parsed.MetadataUserID)
	if uid == nil {
		return ""
	}
	return sanitizeSessionID(uid.SessionID)
}

// ExplicitGatewaySessionID combines the generic gateway metadata session with
// an explicit protocol header. It never consults content-derived hashes.
func ExplicitGatewaySessionID(c *gin.Context, parsed *ParsedRequest) string {
	if sessionID := ExplicitSessionIDFromParsedRequest(parsed); sessionID != "" {
		return sessionID
	}
	return ExtractClientSessionID(c)
}

// maxPersistedSessionIDLength bounds the persisted client session identifier to the
// usage_logs.session_id column width (VARCHAR(255)). Longer values are rejected so
// distinct identifiers can never alias through truncation.
const maxPersistedSessionIDLength = 255

// clientSessionIDHeaders extends the OpenAI-compatible sticky-session signals with
// native protocol identifiers that are safe to persist but must not alter OpenAI
// scheduling behavior.
var clientSessionIDHeaders = append(
	append([]string(nil), explicitOpenAIHeaderSessionNames...),
	claudeCodeSessionHeader,
)

// ClaudeCodeSessionIDFromHeader returns the stable Claude Code conversation
// identifier carried by X-Claude-Code-Session-Id. It is intentionally exposed
// separately from ExtractClientSessionID: callers that use it for routing must
// make that scope explicit rather than accidentally changing every protocol's
// session semantics.
func ClaudeCodeSessionIDFromHeader(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return sanitizeSessionID(c.GetHeader(claudeCodeSessionHeader))
}

// ExtractClientSessionID resolves the explicit client-provided session identifier from
// request headers for usage-log correlation and returns it sanitized. It is
// protocol-agnostic and shared by every gateway handler so all supported protocols
// record session_id through one seam. Returns "" when no valid identifier is present.
//
// This value feeds only usage_logs.session_id persistence. It does NOT affect sticky
// routing, account selection, request_id semantics, or upstream prompt caching, which
// keep their own (intentionally broader) session-signal resolution.
func ExtractClientSessionID(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	for _, header := range clientSessionIDHeaders {
		if sessionID := sanitizeSessionID(c.GetHeader(header)); sessionID != "" {
			return sessionID
		}
	}
	if isGrokRequestContext(c) {
		if sessionID := sanitizeSessionID(c.GetHeader(grokConversationIDHeader)); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

// sanitizeSessionID normalizes a raw client-supplied session identifier for safe
// persistence: it trims surrounding whitespace, rejects the value outright if it
// contains any control character (CR/LF/tab/NUL/…) so a log- or header-injection style
// payload cannot slip into stored correlation data, and rejects values longer than
// the DB column bound. Absent or invalid input yields "".
func sanitizeSessionID(raw string) string {
	if !utf8.ValidString(raw) {
		return ""
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	count := 0
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			// An explicit correlation id never legitimately contains control
			// characters; drop the whole value rather than persist a mangled or
			// partially-injected identifier.
			return ""
		}
		count++
		if count > maxPersistedSessionIDLength {
			return ""
		}
	}
	return trimmed
}
