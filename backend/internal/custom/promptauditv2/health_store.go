package promptauditv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// healthSnapshotRedisKey isolates prompt-audit-v2 health state from other modules.
	healthSnapshotRedisKey = "custom:prompt-audit-v2:health"
)

// ErrHealthSnapshotUnavailable identifies a missing, malformed, version-mismatched,
// or otherwise unusable shared health snapshot.
var ErrHealthSnapshotUnavailable = errors.New("prompt audit v2 health snapshot is unavailable")

// HealthStore persists the shared model-service health snapshot in Redis.
type HealthStore struct {
	rdb *redis.Client
}

// NewHealthStore validates the Redis dependency used as the health authority.
func NewHealthStore(rdb *redis.Client) (*HealthStore, error) {
	if rdb == nil {
		return nil, fmt.Errorf("prompt audit v2 health redis client is required")
	}
	return &HealthStore{rdb: rdb}, nil
}

// SaveSnapshot atomically replaces the current snapshot unless Redis already
// contains a newer version or a newer completed probe for the same version.
// The snapshot has no TTL because periodic probes are intentionally disabled;
// it remains authoritative until configuration changes or a real call updates
// one endpoint's state.
func (s *HealthStore) SaveSnapshot(ctx context.Context, snapshot HealthSnapshot) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("prompt audit v2 health redis client is not initialized")
	}
	if snapshot.ConfigVersion <= 0 || snapshot.CheckedAt.IsZero() {
		return fmt.Errorf("prompt audit v2 health snapshot metadata is invalid")
	}
	if snapshot.Endpoints == nil {
		snapshot.Endpoints = map[string]EndpointRuntime{}
	}
	if snapshot.AvailableEndpoints == nil {
		snapshot.AvailableEndpoints = []string{}
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal prompt audit v2 health snapshot: %w", err)
	}
	err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		current, err := loadHealthSnapshotValue(ctx, tx)
		if err != nil && !errors.Is(err, ErrHealthSnapshotUnavailable) {
			return err
		}
		if err == nil && (current.ConfigVersion > snapshot.ConfigVersion ||
			(current.ConfigVersion == snapshot.ConfigVersion && current.CheckedAt.After(snapshot.CheckedAt))) {
			return nil
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, healthSnapshotRedisKey, payload, 0)
			return nil
		})
		return err
	}, healthSnapshotRedisKey)
	if err != nil {
		return fmt.Errorf("save prompt audit v2 health snapshot: %w", err)
	}
	return nil
}

// LoadSnapshot returns the startup/configuration snapshot for the expected
// configuration version. A missing, malformed, or mismatched snapshot is
// always an explicit error; age alone does not invalidate it.
func (s *HealthStore) LoadSnapshot(ctx context.Context, expectedVersion int64) (HealthSnapshot, error) {
	if s == nil || s.rdb == nil {
		return HealthSnapshot{}, ErrHealthSnapshotUnavailable
	}
	if expectedVersion <= 0 {
		return HealthSnapshot{}, fmt.Errorf("prompt audit v2 health config version is invalid")
	}
	snapshot, err := loadHealthSnapshotValue(ctx, s.rdb)
	if err != nil {
		return HealthSnapshot{}, err
	}
	if snapshot.ConfigVersion != expectedVersion || snapshot.CheckedAt.IsZero() {
		return HealthSnapshot{}, ErrHealthSnapshotUnavailable
	}
	return snapshot, nil
}

// UpdateEndpoint records a real gateway-call result only when the stored
// snapshot still belongs to expectedVersion. A stale caller never overwrites
// a newer probe round or configuration.
func (s *HealthStore) UpdateEndpoint(ctx context.Context, expectedVersion int64, endpointID string, state EndpointRuntime) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("prompt audit v2 health redis client is not initialized")
	}
	if expectedVersion <= 0 || endpointID == "" {
		return fmt.Errorf("prompt audit v2 health endpoint update is invalid")
	}
	if state.CheckedAt.IsZero() {
		state.CheckedAt = time.Now().UTC()
	}
	for attempt := 0; attempt < 3; attempt++ {
		err := s.rdb.Watch(ctx, func(tx *redis.Tx) error {
			snapshot, err := loadHealthSnapshotValue(ctx, tx)
			if err != nil {
				return err
			}
			if snapshot.ConfigVersion != expectedVersion || state.CheckedAt.Before(snapshot.CheckedAt) {
				return nil
			}
			if snapshot.Endpoints == nil {
				snapshot.Endpoints = map[string]EndpointRuntime{}
			}
			snapshot.Endpoints[endpointID] = state
			snapshot.AvailableEndpoints = rebuildAvailableEndpointIDs(snapshot)
			payload, err := json.Marshal(snapshot)
			if err != nil {
				return fmt.Errorf("marshal prompt audit v2 endpoint health update: %w", err)
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, healthSnapshotRedisKey, payload, 0)
				return nil
			})
			return err
		}, healthSnapshotRedisKey)
		if err == nil {
			return nil
		}
		if !errors.Is(err, redis.TxFailedErr) {
			return fmt.Errorf("update prompt audit v2 endpoint health: %w", err)
		}
	}
	return fmt.Errorf("update prompt audit v2 endpoint health: %w", redis.TxFailedErr)
}

// Invalidate removes the shared snapshot when auditing is disabled.
func (s *HealthStore) Invalidate(ctx context.Context) error {
	if s == nil || s.rdb == nil {
		return fmt.Errorf("prompt audit v2 health redis client is not initialized")
	}
	if err := s.rdb.Del(ctx, healthSnapshotRedisKey).Err(); err != nil {
		return fmt.Errorf("invalidate prompt audit v2 health snapshot: %w", err)
	}
	return nil
}

type redisHealthReader interface {
	Get(context.Context, string) *redis.StringCmd
}

func loadHealthSnapshotValue(ctx context.Context, reader redisHealthReader) (HealthSnapshot, error) {
	raw, err := reader.Get(ctx, healthSnapshotRedisKey).Result()
	if errors.Is(err, redis.Nil) {
		return HealthSnapshot{}, ErrHealthSnapshotUnavailable
	}
	if err != nil {
		return HealthSnapshot{}, fmt.Errorf("read prompt audit v2 health snapshot: %w", err)
	}
	var snapshot HealthSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return HealthSnapshot{}, fmt.Errorf("decode prompt audit v2 health snapshot: %w", err)
	}
	if snapshot.Endpoints == nil {
		snapshot.Endpoints = map[string]EndpointRuntime{}
	}
	return snapshot, nil
}

func rebuildAvailableEndpointIDs(snapshot HealthSnapshot) []string {
	available := make([]string, 0, len(snapshot.Endpoints))
	for _, id := range snapshot.AvailableEndpoints {
		if state, ok := snapshot.Endpoints[id]; ok && state.OK {
			available = append(available, id)
		}
	}
	return available
}
