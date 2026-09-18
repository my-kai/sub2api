package turnlog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ErrInvalidCaptureRequest identifies administrator input that must be rejected before dispatch.
var ErrInvalidCaptureRequest = errors.New("invalid turn log capture request")

// AccountReader is the narrow account lookup contract used by manual capture.
type AccountReader interface {
	GetByID(context.Context, int64) (*service.Account, error)
}

// AccountLister is the narrow contract needed to populate the capture selector.
type AccountLister interface {
	ListByPlatform(context.Context, string) ([]service.Account, error)
}

// Service records selected OpenAI OAuth responses and serves administrator queries.
type Service struct {
	store    *Store
	accounts AccountReader
	lister   AccountLister
	proxies  service.ProxyRepository
	gateway  *service.OpenAIGatewayService
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewService creates the manual capture service and retention cleanup worker.
func NewService(store *Store, accounts AccountReader, gateway *service.OpenAIGatewayService, proxies service.ProxyRepository) (*Service, error) {
	if store == nil || accounts == nil {
		return nil, errors.New("turn log store and account reader are required")
	}
	lister, ok := accounts.(AccountLister)
	if !ok {
		return nil, errors.New("turn log account lister is required")
	}
	if gateway == nil {
		return nil, errors.New("turn log gateway is required")
	}
	if proxies == nil {
		return nil, errors.New("turn log proxy repository is required")
	}
	s := &Service{store: store, accounts: accounts, lister: lister, proxies: proxies, gateway: gateway, stop: make(chan struct{}), done: make(chan struct{})}
	go s.run()
	return s, nil
}

// OAuthAccounts returns active OpenAI OAuth accounts without exposing credentials.
func (s *Service) OAuthAccounts(ctx context.Context) ([]OAuthAccount, error) {
	accounts, err := s.lister.ListByPlatform(ctx, service.PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	proxies, err := s.proxies.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	proxyOptions := make([]CaptureProxy, 0, len(proxies))
	now := time.Now()
	for i := range proxies {
		if proxies[i].IsExpired(now) {
			continue
		}
		proxyOptions = append(proxyOptions, captureProxyFromService(&proxies[i]))
	}
	result := make([]OAuthAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.IsOpenAIOAuth() {
			result = append(result, OAuthAccount{ID: account.ID, Name: account.Name, Proxies: proxyOptions})
		}
	}
	return result, nil
}

// Capture performs one administrator-requested upstream capture without persisting it.
func (s *Service) Capture(ctx context.Context, c *gin.Context, accountID int64, model string, proxyID *int64) (*CaptureResult, error) {
	if accountID <= 0 {
		return nil, fmt.Errorf("%w: account is required", ErrInvalidCaptureRequest)
	}
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 200 {
		return nil, fmt.Errorf("%w: model is invalid", ErrInvalidCaptureRequest)
	}
	account, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, service.ErrAccountNotFound) {
			return nil, fmt.Errorf("%w: account not found", ErrInvalidCaptureRequest)
		}
		return nil, err
	}
	if account == nil || !account.IsOpenAIOAuth() {
		return nil, fmt.Errorf("%w: account must be OpenAI OAuth", ErrInvalidCaptureRequest)
	}
	selected := *account.SelectEgressForRequest()
	if proxyID != nil {
		if *proxyID == 0 {
			selected.Proxy = nil
			selected.ProxyID = nil
			selected.SelectedEgressHost = "local"
			selected.SelectedEgressKey = "local"
		} else {
			proxy, proxyErr := s.proxies.GetByID(ctx, *proxyID)
			if proxyErr != nil || proxy == nil || !proxy.IsActive() || proxy.IsExpired(time.Now()) {
				return nil, fmt.Errorf("%w: proxy is unavailable", ErrInvalidCaptureRequest)
			}
			selected.Proxy = proxy
			selected.ProxyID = proxyID
			selected.SelectedEgressHost = proxy.Host
			selected.SelectedEgressKey = fmt.Sprintf("proxy:%d", proxy.ID)
		}
	}
	result, err := s.gateway.CaptureOpenAIChatCompletions(ctx, c, &selected, model)
	if err != nil {
		return nil, err
	}
	return &CaptureResult{
		StatusCode:       result.StatusCode,
		ResponseHeaders:  result.ResponseHeaders,
		ResponseBody:     result.ResponseBody,
		HeadersTruncated: result.HeadersTruncated,
		BodyTruncated:    result.BodyTruncated,
	}, nil
}

func captureProxyFromService(proxy *service.Proxy) CaptureProxy {
	return CaptureProxy{ID: proxy.ID, Name: proxy.Name, Host: proxy.Host, Port: proxy.Port, Protocol: proxy.Protocol}
}

// Close stops the retention cleanup worker.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
}

// List returns a validated administrator page.
func (s *Service) List(ctx context.Context, filter TurnLogFilter) (TurnLogPage, error) {
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 {
		return TurnLogPage{}, errors.New("invalid turn log pagination")
	}
	if filter.StatusCode != nil && !isTurnStatus(*filter.StatusCode) {
		return TurnLogPage{}, errors.New("invalid turn log status code")
	}
	return s.store.list(ctx, filter)
}

// Get returns one complete administrator-visible event.
func (s *Service) Get(ctx context.Context, id int64) (TurnLog, error) {
	if id <= 0 {
		return TurnLog{}, errors.New("invalid turn log id")
	}
	item, err := s.store.get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return TurnLog{}, sql.ErrNoRows
	}
	return item, err
}

// Config returns the current retention policy.
func (s *Service) Config(ctx context.Context) (TurnLogConfig, error) { return s.store.loadConfig(ctx) }

// SaveConfig validates and persists retention policy, then triggers immediate cleanup.
func (s *Service) SaveConfig(ctx context.Context, retentionDays int, updatedBy int64) (TurnLogConfig, error) {
	if retentionDays < MinRetentionDays || retentionDays > MaxRetentionDays {
		return TurnLogConfig{}, fmt.Errorf("retention days must be between %d and %d", MinRetentionDays, MaxRetentionDays)
	}
	config, err := s.store.saveConfig(ctx, retentionDays, updatedBy)
	if err != nil {
		return TurnLogConfig{}, err
	}
	s.cleanup(ctx, config.RetentionDays)
	return config, nil
}

func (s *Service) run() {
	defer close(s.done)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if config, err := s.store.loadConfig(ctx); err == nil {
				s.cleanup(ctx, config.RetentionDays)
			} else {
				slog.Error("turn_log_config_load_failed", "error", err)
			}
			cancel()
		case <-s.stop:
			return
		}
	}
}

func (s *Service) cleanup(ctx context.Context, retentionDays int) {
	if retentionDays < MinRetentionDays || retentionDays > MaxRetentionDays {
		return
	}
	count, err := s.store.deleteExpired(ctx, time.Now().UTC().Add(-time.Duration(retentionDays)*24*time.Hour))
	if err != nil {
		slog.Error("turn_log_cleanup_failed", "retention_days", retentionDays, "error", err)
		return
	}
	if count > 0 {
		slog.Info("turn_log_cleanup_completed", "deleted", count, "retention_days", retentionDays)
	}
}

func isTurnStatus(status int) bool { return status == 217 || status == 292 }

func parsePositiveInt(raw string) (int64, bool) {
	value, err := strconv.ParseInt(raw, 10, 64)
	return value, err == nil && value > 0
}
