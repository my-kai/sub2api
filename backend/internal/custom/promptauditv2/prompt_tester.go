package promptauditv2

import (
	"context"
	"strings"
)

// TestPrompt validates the unsaved prompt, user input, and endpoint draft from
// request, then calls enabled services in priority order under ctx cancellation.
// It returns the first strict result; invalid drafts and exhausted services are
// explicit errors. It never evaluates rules or mutates logs, restrictions,
// notifications, or runtime metrics.
func (s *Service) TestPrompt(ctx context.Context, request PromptTestRequest) (PromptTestResult, error) {
	if strings.Count(request.PromptTemplate, promptPlaceholder) != 1 {
		return PromptTestResult{}, invalidConfig("prompt_template must contain exactly one {{user_input}}")
	}
	if strings.TrimSpace(request.UserInput) == "" {
		return PromptTestResult{}, invalidConfig("user_input must not be empty")
	}

	existing, err := s.store.LoadConfig(ctx)
	if err != nil {
		return PromptTestResult{}, err
	}
	prepared, err := prepareEndpoints(request.Endpoints, &existing, true)
	if err != nil {
		return PromptTestResult{}, err
	}
	active, err := buildActiveEndpoints(prepared)
	if err != nil {
		return PromptTestResult{}, err
	}
	if len(active) == 0 {
		return PromptTestResult{}, invalidConfig("prompt test requires at least one enabled endpoint")
	}

	result, err := NewEndpointPool(s.client, active, nil, nil).Audit(ctx, request.PromptTemplate, request.UserInput)
	if err != nil {
		return PromptTestResult{}, ErrPromptTestUnavailable.WithCause(err)
	}
	return PromptTestResult{
		Confidence: result.Confidence, Reason: result.Reason,
		EndpointName: result.EndpointName, AuditModel: result.AuditModel, LatencyMS: result.LatencyMS,
	}, nil
}
