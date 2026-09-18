package turnlog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// AccountReader is the narrow account lookup contract used by the async worker.
type AccountReader interface {
	GetByID(context.Context, int64) (*service.Account, error)
}

// Service records selected OpenAI OAuth responses and serves administrator queries.
type Service struct {
	store    *Store
	accounts AccountReader
	queue    chan queuedEvent
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewService creates and starts the bounded asynchronous writer and cleanup worker.
func NewService(store *Store, accounts AccountReader) (*Service, error) {
	if store == nil || accounts == nil {
		return nil, errors.New("turn log store and account reader are required")
	}
	s := &Service{store: store, accounts: accounts, queue: make(chan queuedEvent, queueCapacity), stop: make(chan struct{}), done: make(chan struct{})}
	go s.run()
	return s, nil
}

// Close stops the worker after draining queued events within the caller's process lifetime.
func (s *Service) Close() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
}

// ObserveHTTPResponse wraps matching response bodies and leaves all other responses untouched.
func (s *Service) ObserveHTTPResponse(req *http.Request, accountID int64, resp *http.Response) *http.Response {
	if s == nil || resp == nil || accountID <= 0 || !isOpenAIRequest(req) || !isTurnStatus(resp.StatusCode) || resp.Body == nil {
		return resp
	}
	headers, headersTruncated := captureHeaders(resp.Header)
	wrapped := &captureBody{ReadCloser: resp.Body, accountID: accountID, statusCode: resp.StatusCode, headers: headers, headersTruncated: headersTruncated, enqueue: s.enqueue}
	resp.Body = wrapped
	return resp
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

func (s *Service) enqueue(event queuedEvent) {
	select {
	case s.queue <- event:
	default:
		slog.Error("turn_log_queue_full", "account_id", event.AccountID, "status_code", event.StatusCode, "queue_capacity", queueCapacity)
	}
}

func (s *Service) run() {
	defer close(s.done)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case event := <-s.queue:
			s.write(event)
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			if config, err := s.store.loadConfig(ctx); err == nil {
				s.cleanup(ctx, config.RetentionDays)
			} else {
				slog.Error("turn_log_config_load_failed", "error", err)
			}
			cancel()
		case <-s.stop:
			for {
				select {
				case event := <-s.queue:
					s.write(event)
				default:
					return
				}
			}
		}
	}
}

func (s *Service) write(event queuedEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	account, err := s.accounts.GetByID(ctx, event.AccountID)
	if err != nil {
		slog.Error("turn_log_account_lookup_failed", "account_id", event.AccountID, "error", err)
		return
	}
	if account == nil || account.Platform != service.PlatformOpenAI || account.Type != service.AccountTypeOAuth {
		return
	}
	if err := s.store.insert(ctx, event, account.Name); err != nil {
		slog.Error("turn_log_write_failed", "account_id", event.AccountID, "status_code", event.StatusCode, "error", err)
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

func isOpenAIRequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	profile := service.HTTPUpstreamProfileFromContext(req.Context())
	return profile == service.HTTPUpstreamProfileOpenAI
}

func captureHeaders(headers http.Header) (map[string][]string, bool) {
	result := make(map[string][]string, len(headers))
	used := int64(2)
	truncated := false
	for key, values := range headers {
		for _, value := range values {
			entryBytes := int64(len(key) + len(value) + 4)
			if used+entryBytes > MaxHeadersBytes {
				truncated = true
				continue
			}
			result[key] = append(result[key], value)
			used += entryBytes
		}
	}
	return result, truncated
}

func parsePositiveInt(raw string) (int64, bool) {
	value, err := strconv.ParseInt(raw, 10, 64)
	return value, err == nil && value > 0
}
