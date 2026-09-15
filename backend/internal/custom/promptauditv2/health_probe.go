package promptauditv2

import (
	"context"
	"time"
)

const (
	// healthProbeTemplate keeps probes harmless while exercising strict response parsing.
	healthProbeTemplate = `Review {{user_input}} and return exactly {"confidence":0,"reason":"safe probe"}.`
	// healthProbeInput is never persisted and is sent only to configured audit services.
	healthProbeInput = "This is a harmless connection probe."
)

// probeRuntimeEndpoints checks every active endpoint and builds one complete
// snapshot, keeping failed services visible but unavailable for the UI.
func (s *Service) probeRuntimeEndpoints(ctx context.Context, runtime *RuntimeConfig) (HealthSnapshot, error) {
	if runtime == nil || !runtime.Enabled || len(runtime.Endpoints) == 0 {
		return HealthSnapshot{}, ErrHealthSnapshotUnavailable
	}
	snapshot := HealthSnapshot{
		ConfigVersion: runtime.ConfigVersion,
		Endpoints:     make(map[string]EndpointRuntime, len(runtime.Endpoints)),
	}
	for _, endpoint := range runtime.Endpoints {
		if err := ctx.Err(); err != nil {
			return HealthSnapshot{}, err
		}
		startedAt := time.Now()
		result, err := s.client.Audit(ctx, endpoint, healthProbeTemplate, healthProbeInput)
		state := EndpointRuntime{CheckedAt: time.Now().UTC()}
		if err != nil {
			state.Status = endpointErrorCode(err)
		} else {
			state.OK = true
			state.Status = "available"
			state.LatencyMS = result.LatencyMS
		}
		if state.LatencyMS == 0 {
			state.LatencyMS = int(time.Since(startedAt).Milliseconds())
		}
		snapshot.Endpoints[endpoint.ID] = state
		if state.OK {
			snapshot.AvailableEndpoints = append(snapshot.AvailableEndpoints, endpoint.ID)
		}
	}
	snapshot.CheckedAt = time.Now().UTC()
	return snapshot, nil
}
