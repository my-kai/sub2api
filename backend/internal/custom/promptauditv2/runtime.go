package promptauditv2

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
)

// Reload loads and activates the latest persisted configuration. Saving an
// invalid enabled snapshot never leaves the previous policy silently active.
func (s *Service) Reload(ctx context.Context) error {
	config, err := s.store.LoadConfig(ctx)
	if err != nil {
		return err
	}
	return s.activate(config)
}

func (s *Service) activate(config Config) error {
	configuredProtocols := make(map[string]struct{}, len(config.EnabledProtocols))
	for _, protocol := range config.EnabledProtocols {
		configuredProtocols[protocol] = struct{}{}
	}
	runtime, buildErr := BuildRuntimeConfig(config)
	var newPool *WorkerPool
	if buildErr == nil && runtime.Enabled {
		endpointPool := NewEndpointPoolWithHealth(s.client, runtime.Endpoints, func() {
			s.failovers.Add(1)
		}, s.recordEndpointRuntime, s.healthStore, runtime.ConfigVersion, func(error) {
			s.setLastError(ErrorCodeUnavailable)
		})
		newPool, buildErr = NewWorkerPool(runtime.WorkerCount, runtime.QueueCapacity, func(ctx context.Context, request securityaudit.Request, message string) securityaudit.Decision {
			return s.processAudit(runtime, endpointPool, ctx, request, message)
		})
	}

	s.mu.Lock()
	if s.configVersion > 0 && config.ConfigVersion <= s.configVersion {
		s.mu.Unlock()
		if newPool != nil {
			newPool.StopAccepting()
			newPool.Wait()
		}
		return nil
	}
	oldPool := s.pool
	s.configuredEnabled = config.Enabled
	s.configuredProtocols = configuredProtocols
	s.configVersion = config.ConfigVersion
	s.runtime = runtime
	s.pool = newPool
	switch {
	case buildErr != nil:
		s.status = "unavailable"
		s.runtime = nil
	case !config.Enabled:
		s.status = "disabled"
	default:
		s.status = "running"
	}
	s.mu.Unlock()
	s.resetEndpointRuntime(config.Endpoints)
	if config.Enabled && runtime != nil && runtime.Enabled {
		s.signalHealthProbe()
	} else if err := s.healthStore.Invalidate(context.Background()); err != nil {
		s.setLastError(ErrorCodeUnavailable)
	}
	if oldPool != nil {
		oldPool.StopAccepting()
		s.retiredMu.Lock()
		s.retiredPools = append(s.retiredPools, oldPool)
		s.retiredMu.Unlock()
	}
	if buildErr != nil {
		s.setLastError(ErrorCodeInvalidConfig)
		return fmt.Errorf("activate prompt audit v2 config: %w", buildErr)
	}
	return nil
}

// Runtime returns non-secret process and queue state. Persistent email counts
// are read at request time so multi-instance work is represented accurately.
func (s *Service) Runtime(ctx context.Context) RuntimeSnapshot {
	s.mu.RLock()
	status := s.status
	version := s.configVersion
	runtime := s.runtime
	pool := s.pool
	s.mu.RUnlock()
	snapshot := RuntimeSnapshot{Status: status, ConfigVersion: version, Endpoints: map[string]EndpointRuntime{}}
	if runtime != nil && runtime.Enabled {
		health, err := s.healthStore.LoadSnapshot(ctx, version)
		if err != nil {
			snapshot.Status = "unavailable"
			snapshot.HealthErrorCode = ErrorCodeUnavailable
		} else {
			snapshot.HealthConfigVersion = health.ConfigVersion
			checkedAt := health.CheckedAt
			snapshot.HealthCheckedAt = &checkedAt
			snapshot.Endpoints = health.Endpoints
		}
	}
	if runtime != nil {
		snapshot.WorkerCount = runtime.WorkerCount
		snapshot.QueueCapacity = runtime.QueueCapacity
	}
	if pool != nil {
		snapshot.Queued, snapshot.Processing = pool.Stats()
	}
	snapshot.Failovers = s.failovers.Load()
	snapshot.Hits = s.hits.Load()
	snapshot.Rejected = s.rejected.Load()
	snapshot.Unavailable = s.unavailable.Load()
	pending, failed, err := s.store.EmailQueueCounts(ctx)
	if err != nil {
		s.setLastError(ErrorCodePersistence)
	} else {
		snapshot.EmailPending = pending
		snapshot.EmailFailed = failed
	}
	s.lastErrorMu.RLock()
	snapshot.LastErrorCode = s.lastErrorCode
	if s.lastErrorAt != nil {
		value := *s.lastErrorAt
		snapshot.LastErrorAt = &value
	}
	s.lastErrorMu.RUnlock()
	return snapshot
}

// Close stops accepting new model work and waits for accepted and background work.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.backgroundCancel()
	s.mu.RLock()
	pool := s.pool
	s.mu.RUnlock()
	if pool != nil {
		pool.StopAccepting()
	}
	s.retiredMu.Lock()
	retired := append([]*WorkerPool(nil), s.retiredPools...)
	s.retiredMu.Unlock()
	if pool != nil {
		pool.Wait()
	}
	for _, retiredPool := range retired {
		retiredPool.Wait()
	}
	s.background.Wait()
}

func (s *Service) recordEndpointRuntime(id string, state EndpointRuntime) {
	s.endpointMu.Lock()
	s.endpoints[id] = state
	s.endpointMu.Unlock()
}

func (s *Service) resetEndpointRuntime(endpoints []EndpointConfig) {
	s.endpointMu.Lock()
	next := make(map[string]EndpointRuntime, len(endpoints))
	for _, endpoint := range endpoints {
		if !endpoint.Enabled {
			continue
		}
		state, exists := s.endpoints[endpoint.ID]
		if !exists {
			state = EndpointRuntime{Status: "unknown"}
		}
		next[endpoint.ID] = state
	}
	s.endpoints = next
	s.endpointMu.Unlock()
}

func (s *Service) endpointSnapshot() map[string]EndpointRuntime {
	s.endpointMu.RLock()
	defer s.endpointMu.RUnlock()
	result := make(map[string]EndpointRuntime, len(s.endpoints))
	for id, state := range s.endpoints {
		result[id] = state
	}
	return result
}

func (s *Service) startBackgroundWorkers() {
	s.background.Add(4)
	go func() {
		defer s.background.Done()
		s.runEmailWorker(s.backgroundCtx)
	}()
	go func() {
		defer s.background.Done()
		s.runRetentionWorker(s.backgroundCtx)
	}()
	go func() {
		defer s.background.Done()
		s.runConfigWatcher(s.backgroundCtx)
	}()
	go func() {
		defer s.background.Done()
		s.runHealthWorker(s.backgroundCtx)
	}()
}

// signalHealthProbe wakes the health worker after a configuration activation.
func (s *Service) signalHealthProbe() {
	if s == nil || s.healthWake == nil {
		return
	}
	select {
	case s.healthWake <- struct{}{}:
	default:
	}
}

// runHealthWorker performs an explicit probe after startup or configuration
// activation. Runtime health is otherwise changed only by real request errors.
func (s *Service) runHealthWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.healthWake:
			s.runHealthProbeOnce(ctx)
		}
	}
}

func (s *Service) runHealthProbeOnce(ctx context.Context) {
	s.mu.RLock()
	runtime := s.runtime
	enabled := s.configuredEnabled
	s.mu.RUnlock()
	if !enabled || runtime == nil || !runtime.Enabled {
		return
	}
	snapshot, err := s.probeRuntimeEndpoints(ctx, runtime)
	if err != nil {
		if ctx.Err() == nil {
			s.setLastError(ErrorCodeUnavailable)
		}
		return
	}
	if err := s.healthStore.SaveSnapshot(ctx, snapshot); err != nil {
		s.setLastError(ErrorCodeUnavailable)
		return
	}
	for id, state := range snapshot.Endpoints {
		s.recordEndpointRuntime(id, state)
	}
}

func (s *Service) runConfigWatcher(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			config, err := s.store.LoadConfig(ctx)
			if err != nil {
				s.setLastError(ErrorCodeUnavailable)
				continue
			}
			s.mu.RLock()
			currentVersion := s.configVersion
			s.mu.RUnlock()
			if config.ConfigVersion > currentVersion {
				_ = s.activate(config)
			}
		}
	}
}

func (s *Service) runRetentionWorker(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.RLock()
			runtime := s.runtime
			s.mu.RUnlock()
			if runtime == nil || runtime.LogRetentionDays <= 0 {
				continue
			}
			if _, err := s.store.DeleteExpiredEvents(ctx, now.UTC().AddDate(0, 0, -runtime.LogRetentionDays)); err != nil {
				s.setLastError(ErrorCodePersistence)
			}
		}
	}
}
