package promptauditv2

import (
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// AdminHandler exposes configuration, probes, runtime state, and hit-only logs.
type AdminHandler struct{ service *Service }

// NewAdminHandler creates the system-administrator HTTP adapter.
func NewAdminHandler(service *Service) *AdminHandler { return &AdminHandler{service: service} }

// GetConfig returns a secret-redacted complete configuration.
func (h *AdminHandler) GetConfig(c *gin.Context) {
	config, err := h.service.GetConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}

// UpdateConfig validates and atomically replaces the current configuration.
func (h *AdminHandler) UpdateConfig(c *gin.Context) {
	middleware.SetAuditAction(c, "admin.prompt_audit_v2.config.update")
	var request UpdateConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest(ErrorCodeInvalidConfig, "提示词审计2配置请求无效"))
		return
	}
	config, err := h.service.SaveConfig(c.Request.Context(), request, adminUserID(c))
	if err != nil {
		middleware.SetAuditExtra(c, map[string]any{"result": "failed", "error_code": infraerrors.Reason(err)})
		response.ErrorFrom(c, err)
		return
	}
	middleware.SetAuditExtra(c, map[string]any{
		"result": "success", "enabled": config.Enabled, "config_version": config.ConfigVersion,
		"endpoint_count": len(config.Endpoints), "protocol_count": len(config.EnabledProtocols),
	})
	response.Success(c, config)
}

// ProbeEndpoint tests one endpoint without persisting the draft secret.
func (h *AdminHandler) ProbeEndpoint(c *gin.Context) {
	middleware.SetAuditAction(c, "admin.prompt_audit_v2.endpoint.probe")
	var request ProbeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest(ErrorCodeInvalidConfig, "模型服务探测请求无效"))
		return
	}
	result := h.service.Probe(c.Request.Context(), request)
	status := "failed"
	if result.OK {
		status = "success"
	}
	middleware.SetAuditExtra(c, map[string]any{
		"result": status, "error_code": result.ErrorCode, "endpoint_id": request.Endpoint.ID,
		"http_status": result.HTTPStatus, "latency_ms": result.LatencyMS,
	})
	response.Success(c, result)
}

// TestPrompt binds the unsaved administrator draft, runs one side-effect-free
// model audit, and returns its strict result. Malformed drafts and exhausted
// model services are returned as explicit API errors without logging the body.
func (h *AdminHandler) TestPrompt(c *gin.Context) {
	middleware.SetAuditAction(c, "admin.prompt_audit_v2.prompt.test")
	startedAt := time.Now()
	var request PromptTestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		middleware.SetAuditExtra(c, map[string]any{"result": "failed", "error_code": ErrorCodeInvalidConfig})
		response.ErrorFrom(c, infraerrors.BadRequest(ErrorCodeInvalidConfig, "提示词测试请求无效"))
		return
	}

	result, err := h.service.TestPrompt(c.Request.Context(), request)
	latencyMS := int(time.Since(startedAt).Milliseconds())
	if err != nil {
		middleware.SetAuditExtra(c, map[string]any{
			"result": "failed", "error_code": infraerrors.Reason(err),
			"endpoint_count": countEnabledEndpoints(request.Endpoints), "latency_ms": latencyMS,
		})
		response.ErrorFrom(c, err)
		return
	}
	middleware.SetAuditExtra(c, map[string]any{
		"result": "success", "endpoint_count": countEnabledEndpoints(request.Endpoints), "latency_ms": latencyMS,
	})
	response.Success(c, result)
}

// GetRuntime returns non-secret runtime and persistent queue state.
func (h *AdminHandler) GetRuntime(c *gin.Context) {
	response.Success(c, h.service.Runtime(c.Request.Context()))
}

// ListEvents returns only rule-hit rows and never includes full messages.
func (h *AdminHandler) ListEvents(c *gin.Context) {
	page, err := positiveQueryInt(c, "page", 1, 0)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	pageSize, err := positiveQueryInt(c, "page_size", 20, 100)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	filter, err := eventFilter(c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result, err := h.service.ListEvents(c.Request.Context(), filter, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// GetEvent returns one event and decrypts its complete user message on demand.
func (h *AdminHandler) GetEvent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.ErrorFrom(c, infraerrors.BadRequest(ErrorCodeNotFound, "事件 ID 无效"))
		return
	}
	event, err := h.service.GetEvent(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, event)
}

func adminUserID(c *gin.Context) int64 {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		return 0
	}
	return subject.UserID
}

func positiveQueryInt(c *gin.Context, key string, fallback, maximum int) (int, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 || (maximum > 0 && parsed > maximum) {
		return 0, infraerrors.BadRequest(ErrorCodeInvalidConfig, "分页参数无效")
	}
	return parsed, nil
}

func eventFilter(c *gin.Context) (EventFilter, error) {
	filter := EventFilter{Action: normalizeFilterText(c.Query("action")), Protocol: normalizeFilterText(c.Query("protocol"))}
	var err error
	if filter.StartAt, err = optionalTimeQuery(c.Query("start_at")); err != nil {
		return EventFilter{}, err
	}
	if filter.EndAt, err = optionalTimeQuery(c.Query("end_at")); err != nil {
		return EventFilter{}, err
	}
	if filter.UserID, err = optionalInt64Query(c.Query("user_id")); err != nil {
		return EventFilter{}, err
	}
	if filter.MinConfidence, err = optionalFloatQuery(c.Query("min_confidence")); err != nil {
		return EventFilter{}, err
	}
	if filter.MaxConfidence, err = optionalFloatQuery(c.Query("max_confidence")); err != nil {
		return EventFilter{}, err
	}
	return filter, nil
}

func optionalTimeQuery(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, infraerrors.BadRequest(ErrorCodeInvalidConfig, "时间筛选值无效")
	}
	return &value, nil
}

func optionalInt64Query(raw string) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, infraerrors.BadRequest(ErrorCodeInvalidConfig, "用户筛选值无效")
	}
	return &value, nil
}

func optionalFloatQuery(raw string) (*float64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, infraerrors.BadRequest(ErrorCodeInvalidConfig, "置信度筛选值无效")
	}
	return &value, nil
}
