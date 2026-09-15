package promptauditv2

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// EndpointPool calls configured services in descending priority order and only
// fails the request after every enabled service has produced an explicit error.
type EndpointPool struct {
	client        *OpenAIClient
	endpoints     []ActiveEndpoint
	onFailover    func()
	onResult      func(string, EndpointRuntime)
	health        *HealthStore
	version       int64
	onHealthError func(error)
}

// NewEndpointPool freezes endpoint order for one runtime configuration version.
func NewEndpointPool(client *OpenAIClient, endpoints []ActiveEndpoint, onFailover func(), onResult func(string, EndpointRuntime)) *EndpointPool {
	return NewEndpointPoolWithHealth(client, endpoints, onFailover, onResult, nil, 0, nil)
}

// NewEndpointPoolWithHealth creates a pool that filters calls through the
// shared Redis health snapshot for one immutable configuration version.
func NewEndpointPoolWithHealth(client *OpenAIClient, endpoints []ActiveEndpoint, onFailover func(), onResult func(string, EndpointRuntime), health *HealthStore, version int64, onHealthError func(error)) *EndpointPool {
	return &EndpointPool{
		client: client, endpoints: append([]ActiveEndpoint(nil), endpoints...),
		onFailover: onFailover, onResult: onResult, health: health, version: version,
		onHealthError: onHealthError,
	}
}

// Audit returns the first valid strict result. Errors never become an allow decision.
func (p *EndpointPool) Audit(ctx context.Context, template, input string) (AuditResult, error) {
	if p == nil || p.client == nil || len(p.endpoints) == 0 {
		return AuditResult{}, &EndpointCallError{Code: ErrorCodeUnavailable, Cause: errors.New("no audit endpoint is available")}
	}
	endpoints, err := p.availableEndpoints(ctx)
	if err != nil {
		return AuditResult{}, err
	}
	var lastErr error
	for index, endpoint := range endpoints {
		result, err := p.client.Audit(ctx, endpoint, template, input)
		if err == nil {
			return result, nil
		}
		if p.onResult != nil {
			p.onResult(endpoint.ID, EndpointRuntime{OK: false, Status: endpointErrorCode(err), CheckedAt: resultTime()})
		}
		p.updateHealth(ctx, endpoint.ID, EndpointRuntime{OK: false, Status: endpointErrorCode(err), CheckedAt: resultTime()})
		lastErr = err
		if index < len(endpoints)-1 && p.onFailover != nil {
			p.onFailover()
		}
		if ctx.Err() != nil {
			return AuditResult{}, ctx.Err()
		}
	}
	return AuditResult{}, fmt.Errorf("all prompt audit v2 endpoints failed: %w", lastErr)
}

func (p *EndpointPool) availableEndpoints(ctx context.Context) ([]ActiveEndpoint, error) {
	if p.health == nil {
		return p.endpoints, nil
	}
	snapshot, err := p.health.LoadSnapshot(ctx, p.version)
	if err != nil {
		return nil, &EndpointCallError{Code: ErrorCodeUnavailable, Cause: err}
	}
	byID := make(map[string]ActiveEndpoint, len(p.endpoints))
	for _, endpoint := range p.endpoints {
		byID[endpoint.ID] = endpoint
	}
	available := make([]ActiveEndpoint, 0, len(snapshot.AvailableEndpoints))
	for _, id := range snapshot.AvailableEndpoints {
		if endpoint, ok := byID[id]; ok {
			available = append(available, endpoint)
		}
	}
	if len(available) == 0 {
		return nil, &EndpointCallError{Code: ErrorCodeUnavailable, Cause: errors.New("no healthy audit endpoint is available")}
	}
	return available, nil
}

func (p *EndpointPool) updateHealth(ctx context.Context, id string, state EndpointRuntime) {
	if p.health == nil {
		return
	}
	if err := p.health.UpdateEndpoint(ctx, p.version, id, state); err != nil && p.onHealthError != nil {
		p.onHealthError(err)
	}
}

func endpointErrorCode(err error) string {
	var endpointErr *EndpointCallError
	if errors.As(err, &endpointErr) && endpointErr.Code != "" {
		return endpointErr.Code
	}
	return ErrorCodeUnavailable
}

func resultTime() time.Time { return time.Now().UTC() }
