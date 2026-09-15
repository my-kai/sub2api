package promptauditv2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	mainservice "github.com/Wei-Shaw/sub2api/internal/service"
)

// Service is both the gateway preflight engine and the owner of hot-swappable
// audit worker generations.
type Service struct {
	store        *Store
	healthStore  *HealthStore
	userService  *mainservice.UserService
	emailService *mainservice.EmailService
	client       *OpenAIClient

	mu                  sync.RWMutex
	runtime             *RuntimeConfig
	pool                *WorkerPool
	configuredEnabled   bool
	configuredProtocols map[string]struct{}
	configVersion       int64
	status              string
	retiredMu           sync.Mutex
	retiredPools        []*WorkerPool

	endpointMu    sync.RWMutex
	endpoints     map[string]EndpointRuntime
	failovers     atomic.Int64
	hits          atomic.Int64
	rejected      atomic.Int64
	unavailable   atomic.Int64
	lastErrorMu   sync.RWMutex
	lastErrorCode string
	lastErrorAt   *time.Time

	backgroundCtx    context.Context
	backgroundCancel context.CancelFunc
	background       sync.WaitGroup
	healthWake       chan struct{}
}

// NewService loads the persisted gate configuration and starts persistent
// delivery and retention workers. A corrupt enabled runtime remains fail-closed.
func NewService(store *Store, healthStore *HealthStore, userService *mainservice.UserService, emailService *mainservice.EmailService) (*Service, error) {
	if store == nil || healthStore == nil || userService == nil || emailService == nil {
		return nil, fmt.Errorf("prompt audit v2 service dependencies are required")
	}
	backgroundCtx, cancel := context.WithCancel(context.Background())
	service := &Service{
		store: store, healthStore: healthStore, userService: userService, emailService: emailService,
		client: NewOpenAIClient(), endpoints: make(map[string]EndpointRuntime),
		status: "unavailable", backgroundCtx: backgroundCtx, backgroundCancel: cancel, healthWake: make(chan struct{}, 1),
	}
	config, err := store.LoadConfig(context.Background())
	if err != nil {
		cancel()
		return nil, err
	}
	// Activation errors do not keep the process from serving the management page.
	// The saved enabled protocols remain fail-closed until an administrator fixes them.
	_ = service.activate(config)
	service.startBackgroundWorkers()
	service.signalHealthProbe()
	return service, nil
}

// Check bypasses the module completely when disabled; otherwise it performs
// restriction preflight and sends enabled chat requests through the bounded
// worker generation.
func (s *Service) Check(ctx context.Context, request securityaudit.Request) securityaudit.Decision {
	// The module switch is checked before restrictions so disabling prompt audit
	// immediately stops both model auditing and enforcement from prior hits.
	s.mu.RLock()
	auditEnabled := s.configuredEnabled
	s.mu.RUnlock()
	if !auditEnabled {
		return allowV2Decision()
	}

	now := time.Now().UTC()
	restriction, err := s.store.ActiveRestriction(ctx, request.UserID, now)
	if err != nil {
		return s.unavailableDecision(ErrorCodeUnavailable)
	}
	if restriction != nil {
		s.rejected.Add(1)
		if restriction.Action == ActionBan {
			return rejectDecision(http.StatusForbidden, ErrorCodeUserBanned, "账号已被安全策略禁用")
		}
		return rejectDecision(http.StatusForbidden, ErrorCodeUserRestricted, "账号当前被安全策略限制，请稍后重试")
	}

	s.mu.RLock()
	_, protocolEnabled := s.configuredProtocols[request.Protocol]
	runtime := s.runtime
	pool := s.pool
	s.mu.RUnlock()
	if !protocolEnabled {
		return allowV2Decision()
	}
	if runtime == nil || pool == nil {
		return s.unavailableDecision(ErrorCodeUnavailable)
	}
	message, present, err := ExtractLatestUserMessage(request.Protocol, request.Body)
	if err != nil {
		s.setLastError(ErrorCodeUnavailable)
		return s.unavailableDecision(ErrorCodeUnavailable)
	}
	if !present {
		return allowV2Decision()
	}
	decision, err := pool.Submit(ctx, request, message)
	if err == nil {
		return decision
	}
	if errors.Is(err, errWorkerQueueFull) {
		s.setLastError(ErrorCodeQueueFull)
		return s.unavailableDecision(ErrorCodeQueueFull)
	}
	s.setLastError(ErrorCodeUnavailable)
	return s.unavailableDecision(ErrorCodeUnavailable)
}

func (s *Service) processAudit(runtime *RuntimeConfig, endpoints *EndpointPool, ctx context.Context, request securityaudit.Request, message string) securityaudit.Decision {
	result, err := endpoints.Audit(ctx, runtime.PromptTemplate, message)
	if err != nil {
		s.setLastError(ErrorCodeUnavailable)
		return s.unavailableDecision(ErrorCodeUnavailable)
	}
	if !MatchRule(runtime.Rule, result.Confidence) {
		return allowV2Decision()
	}
	if ctx.Err() != nil {
		return s.unavailableDecision(ErrorCodeUnavailable)
	}
	outcome, err := s.store.RecordHit(ctx, HitInput{
		RequestID: request.RequestID, UserID: request.UserID, Username: request.Username,
		UserEmail: request.UserEmail, APIKeyID: request.APIKeyID, APIKeyName: request.APIKeyName,
		Protocol: request.Protocol, RequestModel: request.Model, Message: message,
		Result: result, Rule: runtime.Rule,
	})
	if err != nil {
		s.setLastError(ErrorCodePersistence)
		return s.unavailableDecision(ErrorCodePersistence)
	}
	s.hits.Add(1)
	if !outcome.ThresholdReached {
		return allowV2Decision()
	}
	if outcome.Action == ActionBan {
		// RecordHit already changed the authoritative row inside its transaction.
		// Reusing UpdateStatus refreshes the existing auth-cache invalidation path.
		if err := s.userService.UpdateStatus(ctx, request.UserID, mainservice.StatusDisabled); err != nil {
			s.setLastError(ErrorCodePersistence)
			return s.unavailableDecision(ErrorCodePersistence)
		}
	}
	s.rejected.Add(1)
	return rejectDecision(http.StatusForbidden, ErrorCodeRuleTriggered, "请求触发安全规则，已限制当前账号")
}

func allowV2Decision() securityaudit.Decision {
	return securityaudit.Decision{Kind: securityaudit.DecisionAllow, HTTPStatus: http.StatusOK, AllowNextStage: true}
}

func rejectDecision(status int, code, message string) securityaudit.Decision {
	return securityaudit.Decision{
		Kind: securityaudit.DecisionBlock, HTTPStatus: status,
		ErrorCode: code, ClientMessage: message, AllowNextStage: false,
	}
}

func (s *Service) unavailableDecision(code string) securityaudit.Decision {
	s.unavailable.Add(1)
	return securityaudit.Decision{
		Kind: securityaudit.DecisionUnavailable, HTTPStatus: http.StatusServiceUnavailable,
		ErrorCode: code, ClientMessage: "审计服务不可用，请稍后重试", AllowNextStage: false,
	}
}

func (s *Service) setLastError(code string) {
	now := time.Now().UTC()
	s.lastErrorMu.Lock()
	s.lastErrorCode = code
	s.lastErrorAt = &now
	s.lastErrorMu.Unlock()
}
