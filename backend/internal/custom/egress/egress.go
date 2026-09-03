// Package egress contains account-scoped proxy exit configuration helpers.
package egress

import (
	"fmt"
	"sort"
)

const (
	// LocalHost is the stable display and audit label for direct connections.
	LocalHost       = "local"
	proxyIDsKey     = "egress_proxy_ids"
	includeLocalKey = "egress_include_local"
)

// Capacity describes the share of an account's concurrency assigned to one exit.
type Capacity struct {
	Host     string `json:"host"`
	Capacity int    `json:"capacity"`
}

// ConfigFromExtra reads the explicit custom exit configuration from account extra.
func ConfigFromExtra(extra map[string]any) (ids []int64, includeLocal bool, configured bool, err error) {
	includeLocal = true
	if extra == nil {
		return nil, includeLocal, false, nil
	}
	if raw, ok := extra[includeLocalKey]; ok {
		v, ok := raw.(bool)
		if !ok {
			return nil, false, true, fmt.Errorf("%s must be boolean", includeLocalKey)
		}
		includeLocal = v
	}
	raw, ok := extra[proxyIDsKey]
	if !ok {
		return nil, includeLocal, false, nil
	}
	configured = true
	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			n, ok := value.(float64)
			if !ok || n <= 0 || n != float64(int64(n)) {
				return nil, false, true, fmt.Errorf("%s must contain positive integers", proxyIDsKey)
			}
			ids = append(ids, int64(n))
		}
	case []int64:
		ids = append(ids, values...)
	default:
		return nil, false, true, fmt.Errorf("%s must be an array", proxyIDsKey)
	}
	seen := map[int64]struct{}{}
	out := ids[:0]
	for _, id := range ids {
		if id <= 0 {
			return nil, false, true, fmt.Errorf("%s contains invalid proxy id", proxyIDsKey)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, includeLocal, configured, nil
}

// Capacities deterministically distributes total concurrency across exits.
func Capacities(total int, hosts []string) []Capacity {
	if len(hosts) == 0 {
		return nil
	}
	if total < 0 {
		total = 0
	}
	base, remainder := total/len(hosts), total%len(hosts)
	out := make([]Capacity, len(hosts))
	for i, host := range hosts {
		extra := 0
		if i < remainder {
			extra = 1
		}
		out[i] = Capacity{Host: host, Capacity: base + extra}
	}
	return out
}

// SortedProxyIDs returns a stable copy for API responses and persistence normalization.
func SortedProxyIDs(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	sort.SliceStable(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
