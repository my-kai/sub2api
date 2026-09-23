package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EgressStickySessionTTL is the fixed sliding window for explicit session
// to-egress bindings. It intentionally does not reuse account sticky TTLs.
const EgressStickySessionTTL = time.Hour

// ErrEgressStickySessionNotFound distinguishes a cache miss from a cache
// failure so the scheduler can reselect only for a genuine miss.
var ErrEgressStickySessionNotFound = errors.New("egress sticky session not found")

// EgressStickyCache is intentionally separate from GatewayCache. Existing
// account-sticky implementations and their test doubles must not gain new
// methods or accidentally change the account-sticky contract.
type EgressStickyCache interface {
	GetSessionEgressBinding(ctx context.Context, groupID int64, sessionID string) (EgressStickyBinding, error)
	SetSessionEgressBinding(ctx context.Context, groupID int64, sessionID string, binding EgressStickyBinding, ttl time.Duration) error
	RefreshSessionEgressTTL(ctx context.Context, groupID int64, sessionID string, ttl time.Duration) error
}

// EgressStickyBinding stores both the owning account and the request-level
// exit identity; proxy IDs alone are unsafe when accounts have different pools.
type EgressStickyBinding struct {
	AccountID int64  `json:"account_id"`
	EgressKey string `json:"egress_key"`
}

type egressStickyGroupIDContextKey struct{}

// withEgressStickyGroupID keeps the resolved scheduling group beside the
// request so prepareAccountSlot can use the same group namespace.
func withEgressStickyGroupID(ctx context.Context, groupID *int64) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, egressStickyGroupIDContextKey{}, derefGroupID(groupID))
}

func egressStickyGroupIDFromContext(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	groupID, _ := ctx.Value(egressStickyGroupIDContextKey{}).(int64)
	return groupID
}

// resolveEgressStickySelection resolves a request exit before its concurrency
// slot is acquired. A stale binding is ignored and replaced by normal valid
// exit selection; cache read failures remain explicit errors.
func resolveEgressStickySelection(ctx context.Context, cache GatewayCache, account *Account) (*Account, string, bool, error) {
	if account == nil {
		return nil, "", false, fmt.Errorf("account is required")
	}

	sessionID := ExplicitSessionIDFromContext(ctx)
	if sessionID == "" {
		return selectNormalEgress(account)
	}
	egressCache, ok := cache.(EgressStickyCache)
	if !ok {
		return nil, "", false, errors.New("gateway cache unavailable for egress sticky session")
	}

	binding, err := egressCache.GetSessionEgressBinding(ctx, egressStickyGroupIDFromContext(ctx), sessionID)
	if err != nil && !errors.Is(err, ErrEgressStickySessionNotFound) {
		return nil, "", false, fmt.Errorf("read egress sticky session: %w", err)
	}
	if err == nil && binding.AccountID == account.ID {
		if bound := account.selectEgressByKey(binding.EgressKey); bound != nil {
			return bound, binding.EgressKey, true, nil
		}
	}

	bound, key, _, selectErr := selectNormalEgress(account)
	return bound, key, false, selectErr
}

func selectNormalEgress(account *Account) (*Account, string, bool, error) {
	if !account.HasConfiguredEgress() {
		return account, "local", false, nil
	}
	bound := account.SelectEgressForRequest()
	if bound == nil || bound.SelectedEgressKey == "" {
		return nil, "", false, errors.New("no valid egress proxy available")
	}
	return bound, bound.SelectedEgressKey, false, nil
}

// persistEgressStickySelection writes or refreshes the independent binding
// after an exit has been selected, including a wait plan. The caller must pass
// the key that will be used by the reserved or pending slot.
func persistEgressStickySelection(ctx context.Context, cache GatewayCache, accountID int64, egressKey string, stickyHit bool) error {
	sessionID := ExplicitSessionIDFromContext(ctx)
	if sessionID == "" {
		return nil
	}
	egressCache, ok := cache.(EgressStickyCache)
	if !ok {
		return errors.New("gateway cache unavailable for egress sticky session")
	}
	if accountID <= 0 || strings.TrimSpace(egressKey) == "" {
		return errors.New("invalid egress sticky binding")
	}
	binding := EgressStickyBinding{AccountID: accountID, EgressKey: egressKey}
	groupID := egressStickyGroupIDFromContext(ctx)
	if stickyHit {
		return egressCache.RefreshSessionEgressTTL(ctx, groupID, sessionID, EgressStickySessionTTL)
	}
	return egressCache.SetSessionEgressBinding(ctx, groupID, sessionID, binding, EgressStickySessionTTL)
}

// prepareEgressForForward revalidates the request exit at the forwarding seam.
// This covers selection paths that do not acquire a concurrency slot, such as
// count_tokens, while preserving a scheduler-selected key when it is valid.
func prepareEgressForForward(ctx context.Context, cache GatewayCache, account *Account) (*Account, error) {
	if account == nil {
		return nil, errors.New("account is required")
	}
	if account.SelectedEgressKey != "" {
		bound := account.SelectEgressForRequest()
		if bound == nil {
			return nil, errors.New("selected egress is no longer valid")
		}
		return bound, nil
	}
	if !account.HasConfiguredEgress() {
		return account, nil
	}
	bound, egressKey, stickyHit, err := resolveEgressStickySelection(ctx, cache, account)
	if err != nil {
		return nil, err
	}
	if err := persistEgressStickySelection(ctx, cache, bound.ID, egressKey, stickyHit); err != nil {
		return nil, err
	}
	return bound, nil
}

// selectedEgressProxyID parses the stable proxy:<id> key used by slot
// accounting. Local traffic is represented by the literal local key.
func selectedEgressProxyID(key string) (int64, bool) {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, "proxy:") {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(key, "proxy:"), 10, 64)
	return id, err == nil && id > 0
}
