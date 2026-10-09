package egress

import (
	"fmt"
	"time"
)

// ProxyUsable is the shared runtime availability rule. A soft-deleted proxy
// must be rejected even if its stored status is still active; expiry is checked
// at hydration as well as at request selection. Display-only fallback origins
// are not exits and therefore never participate in an account's availability.
func ProxyUsable(status string, deletedAt, expiresAt *time.Time, now time.Time) bool {
	return status == "active" && deletedAt == nil && (expiresAt == nil || expiresAt.After(now))
}

// ResolvePool projects configured exits onto a caller-supplied proxy map.
// Runtime callers supply only usable proxies; administrative callers supply
// all existing proxies so stale bindings remain visible and repairable. The
// configured IDs are retained independently of the resolved proxies: a missing
// legacy proxy must never be reinterpreted as an unconfigured/direct account.
//
// T keeps this helper independent of the service package (which already imports
// egress), avoiding an import cycle. The caller must not put nil values in the
// map. Missing rows are isolated per exit; malformed configuration remains an
// error rather than producing a misleading successful partial snapshot.
func ResolvePool[T any](extra map[string]any, legacyProxyID *int64, proxies map[int64]T) (ids []int64, includeLocal bool, resolved []T, err error) {
	ids, includeLocal, configured, err := ConfigFromExtra(extra)
	if err != nil {
		return nil, false, nil, err
	}
	if configured && len(ids) == 0 && !includeLocal {
		return nil, false, nil, fmt.Errorf("egress_proxy_ids must contain at least one proxy when configured")
	}
	if !configured {
		includeLocal = legacyProxyID == nil
		if legacyProxyID != nil {
			ids = []int64{*legacyProxyID}
		}
	}
	// Copy before exposing the result: ConfigFromExtra accepts []int64 from
	// callers as well as JSON-decoded arrays, and runtime hydration must not
	// share mutable exit IDs with the persisted account configuration.
	ids = append([]int64(nil), ids...)
	for _, id := range ids {
		if proxy, ok := proxies[id]; ok {
			resolved = append(resolved, proxy)
		}
	}
	return ids, includeLocal, resolved, nil
}
