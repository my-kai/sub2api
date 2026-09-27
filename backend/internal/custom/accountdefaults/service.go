package accountdefaults

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ErrInvalidConfig identifies administrator input that must be rejected before persist.
var ErrInvalidConfig = errors.New("invalid account defaults config")

// ProxyPool is the narrow proxy lookup contract needed to resolve defaults:
// only currently available (active, unexpired) proxies are eligible for pre-fill.
// service.ProxyRepository satisfies it structurally; the narrow surface keeps
// tests and future call-sites decoupled from the full repository.
type ProxyPool interface {
	ListActive(ctx context.Context) ([]service.Proxy, error)
	ListByIDs(ctx context.Context, ids []int64) ([]service.Proxy, error)
}

// Service resolves administrator defaults for the create-account form.
type Service struct {
	store   *Store
	proxies ProxyPool
}

// NewService builds the account-defaults service.
func NewService(store *Store, proxies ProxyPool) (*Service, error) {
	if store == nil || proxies == nil {
		return nil, errors.New("account defaults store and proxy pool are required")
	}
	return &Service{store: store, proxies: proxies}, nil
}

// Config returns the persisted administrator configuration.
func (s *Service) Config(ctx context.Context) (Config, error) {
	return s.store.loadConfig(ctx)
}

// SaveConfig validates and persists the administrator configuration.
func (s *Service) SaveConfig(ctx context.Context, cfg Config, updatedBy int64) (Config, error) {
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return s.store.saveConfig(ctx, cfg, updatedBy)
}

// resolve 返回给新建账号表单的默认值。随机模式在表单打开时抽一次（2A 语义），
// 固定模式带入仍可用的配置代理；已失效的代理静默丢弃，避免表单带出不可用出口。
func (s *Service) resolve(ctx context.Context) (ResolvedDefaults, error) {
	cfg, err := s.store.loadConfig(ctx)
	if err != nil {
		return ResolvedDefaults{}, err
	}
	resolved := ResolvedDefaults{
		ModelsByPlatform: cfg.ModelsByPlatform,
		ProxyMode:        cfg.ProxyMode,
		EgressProxyIDs:   []int64{},
	}
	switch cfg.ProxyMode {
	case ProxyModeRandom:
		available, err := s.proxies.ListActive(ctx)
		if err != nil {
			return ResolvedDefaults{}, fmt.Errorf("list available proxies: %w", err)
		}
		if len(available) > 0 {
			// 一次性抽样；表单提交前管理员仍可改。
			pick := available[rand.IntN(len(available))]
			resolved.EgressProxyIDs = []int64{pick.ID}
		}
	case ProxyModeFixed:
		if len(cfg.ProxyFixedIDs) > 0 {
			found, err := s.proxies.ListByIDs(ctx, cfg.ProxyFixedIDs)
			if err != nil {
				return ResolvedDefaults{}, fmt.Errorf("load configured proxies: %w", err)
			}
			alive := make(map[int64]struct{}, len(found))
			for i := range found {
				if isAvailable(&found[i]) {
					alive[found[i].ID] = struct{}{}
				}
			}
			// 保持管理员配置顺序；失效的 ID 跳过。
			for _, id := range cfg.ProxyFixedIDs {
				if _, ok := alive[id]; ok {
					resolved.EgressProxyIDs = append(resolved.EgressProxyIDs, id)
				}
			}
		}
		resolved.EgressIncludeLocal = cfg.AllowLocalEgress
	}
	return resolved, nil
}

// validate enforces the mode-specific shape before persist.
func validate(cfg Config) error {
	switch cfg.ProxyMode {
	case ProxyModeFixed, ProxyModeRandom:
	case "":
		// 未启用代理默认带入。
		return nil
	default:
		return fmt.Errorf("%w: unknown proxy mode %q", ErrInvalidConfig, cfg.ProxyMode)
	}
	if cfg.ProxyMode == ProxyModeFixed && len(cfg.ProxyFixedIDs) == 0 && !cfg.AllowLocalEgress {
		// 固定模式至少要有一个出口（代理或本地直连），否则表单必然带入空出口。
		return fmt.Errorf("%w: fixed proxy mode requires at least one proxy or local egress", ErrInvalidConfig)
	}
	return nil
}

// isAvailable mirrors service-level proxy availability: active and not expired.
func isAvailable(p *service.Proxy) bool {
	if p == nil || !p.IsActive() {
		return false
	}
	if p.ExpiresAt != nil && !p.ExpiresAt.IsZero() && p.ExpiresAt.Before(time.Now()) {
		return false
	}
	return strings.TrimSpace(p.Host) != ""
}
