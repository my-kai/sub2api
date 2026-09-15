package promptauditv2

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GetConfig returns the complete administrator configuration without endpoint secrets.
func (s *Service) GetConfig(ctx context.Context) (Config, error) {
	config, err := s.store.LoadConfig(ctx)
	if err != nil {
		return Config{}, err
	}
	return PublicConfig(config), nil
}

// SaveConfig commits a full CAS update and activates the same persisted version.
func (s *Service) SaveConfig(ctx context.Context, request UpdateConfigRequest, adminID int64) (Config, error) {
	config, err := s.store.SaveConfig(ctx, request, adminID)
	if err != nil {
		return Config{}, err
	}
	if err := s.activate(config); err != nil {
		return Config{}, err
	}
	return PublicConfig(config), nil
}

// Probe validates and tests one draft endpoint with a fixed harmless input.
func (s *Service) Probe(ctx context.Context, request ProbeRequest) ProbeResult {
	checkedAt := time.Now().UTC()
	existing, err := s.store.LoadConfig(ctx)
	if err != nil {
		return ProbeResult{Status: "failed", ErrorCode: ErrorCodeUnavailable, CheckedAt: checkedAt}
	}
	endpoints, err := prepareEndpoints([]EndpointConfig{request.Endpoint}, &existing, true)
	if err != nil || len(endpoints) != 1 || endpoints[0].APIKey == "" {
		return ProbeResult{Status: "failed", ErrorCode: ErrorCodeInvalidConfig, CheckedAt: checkedAt}
	}
	endpoint := endpoints[0]
	endpointURL, err := normalizeChatCompletionsURL(endpoint.BaseURL)
	if err != nil {
		return ProbeResult{Status: "failed", ErrorCode: ErrorCodeInvalidConfig, CheckedAt: checkedAt}
	}
	active := ActiveEndpoint{
		ID: endpoint.ID, Name: endpoint.Name, URL: endpointURL, APIKey: endpoint.APIKey,
		Model: endpoint.Model, Timeout: time.Duration(endpoint.TimeoutMS) * time.Millisecond,
	}
	result, err := s.client.Audit(ctx, active,
		`Review {{user_input}} and return exactly {"confidence":0,"reason":"safe probe"}.`,
		"This is a harmless connection probe.")
	if err != nil {
		probe := ProbeResult{Status: "failed", ErrorCode: endpointErrorCode(err), CheckedAt: checkedAt}
		if endpointErr, ok := err.(*EndpointCallError); ok {
			probe.HTTPStatus = endpointErr.HTTPStatus
		}
		s.recordEndpointRuntime(endpoint.ID, EndpointRuntime{OK: false, Status: probe.ErrorCode, CheckedAt: checkedAt})
		return probe
	}
	probe := ProbeResult{OK: true, Status: "available", HTTPStatus: 200, LatencyMS: result.LatencyMS, CheckedAt: checkedAt}
	s.recordEndpointRuntime(endpoint.ID, EndpointRuntime{OK: true, Status: probe.Status, LatencyMS: probe.LatencyMS, CheckedAt: checkedAt})
	return probe
}

// ListEvents validates pagination and delegates the allowlisted hit-only query.
func (s *Service) ListEvents(ctx context.Context, filter EventFilter, page, pageSize int) (*EventPage, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return nil, invalidConfig("event pagination is invalid")
	}
	if filter.MinConfidence != nil && (*filter.MinConfidence < 0 || *filter.MinConfidence > 1) {
		return nil, invalidConfig("minimum confidence is out of range")
	}
	if filter.MaxConfidence != nil && (*filter.MaxConfidence < 0 || *filter.MaxConfidence > 1) {
		return nil, invalidConfig("maximum confidence is out of range")
	}
	if filter.MinConfidence != nil && filter.MaxConfidence != nil && *filter.MinConfidence > *filter.MaxConfidence {
		return nil, invalidConfig("minimum confidence exceeds maximum confidence")
	}
	if filter.StartAt != nil && filter.EndAt != nil && filter.StartAt.After(*filter.EndAt) {
		return nil, invalidConfig("event start time exceeds end time")
	}
	if filter.Action != "" && filter.Action != "none" && filter.Action != ActionBan && filter.Action != ActionWarning {
		return nil, invalidConfig("event action filter is invalid")
	}
	if filter.Protocol != "" {
		if _, err := validateProtocols([]string{filter.Protocol}); err != nil {
			return nil, err
		}
	}
	return s.store.ListEvents(ctx, filter, page, pageSize)
}

// GetEvent decrypts the full hit message only for the authenticated administrator detail call.
func (s *Service) GetEvent(ctx context.Context, id int64) (*Event, error) {
	if id <= 0 {
		return nil, ErrEventNotFound
	}
	event, err := s.store.GetEvent(ctx, id)
	if err != nil {
		return nil, err
	}
	message, err := s.store.encryptor.Decrypt(event.MessageCiphertext)
	if err != nil {
		return nil, ErrEventDecrypt.WithCause(fmt.Errorf("decrypt prompt audit v2 event %d: %w", id, err))
	}
	event.MessageCiphertext = ""
	event.Message = message
	return event, nil
}

func normalizeFilterText(value string) string { return strings.TrimSpace(value) }
