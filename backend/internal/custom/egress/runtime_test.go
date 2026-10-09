package egress

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProxyUsable(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Second), now.Add(time.Second)
	for _, test := range []struct {
		name      string
		status    string
		deletedAt *time.Time
		expiresAt *time.Time
		want      bool
	}{
		{name: "active", status: "active", want: true},
		{name: "future expiry", status: "active", expiresAt: &future, want: true},
		{name: "inactive", status: "inactive"},
		{name: "deleted", status: "active", deletedAt: &past},
		{name: "expired", status: "active", expiresAt: &past},
		{name: "expiry boundary", status: "active", expiresAt: &now},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, ProxyUsable(test.status, test.deletedAt, test.expiresAt, now))
		})
	}
}

func TestResolvePool(t *testing.T) {
	legacyID := int64(10)
	proxies := map[int64]string{20: "healthy"}
	for _, test := range []struct {
		name         string
		extra        map[string]any
		legacyID     *int64
		wantIDs      []int64
		wantLocal    bool
		wantResolved []string
	}{
		{name: "implicit local", wantLocal: true},
		{name: "missing legacy binding", legacyID: &legacyID, wantIDs: []int64{10}},
		{name: "explicit overrides legacy", legacyID: &legacyID,
			extra:   map[string]any{"egress_proxy_ids": []int64{10, 20, 30}, "egress_include_local": false},
			wantIDs: []int64{10, 20, 30}, wantResolved: []string{"healthy"}},
		{name: "explicit local with missing proxy",
			extra:   map[string]any{"egress_proxy_ids": []any{float64(10)}, "egress_include_local": true},
			wantIDs: []int64{10}, wantLocal: true},
		{name: "explicit local only",
			extra: map[string]any{"egress_proxy_ids": []int64{}}, wantLocal: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ids, local, resolved, err := ResolvePool(test.extra, test.legacyID, proxies)
			require.NoError(t, err)
			require.Equal(t, test.wantIDs, ids)
			require.Equal(t, test.wantLocal, local)
			require.Equal(t, test.wantResolved, resolved)
		})
	}
}

func TestResolvePool_DoesNotShareConfiguredIDs(t *testing.T) {
	configured := []int64{20, 10}
	ids, _, resolved, err := ResolvePool(map[string]any{"egress_proxy_ids": configured}, nil, map[int64]string{10: "second", 20: "first"})
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, resolved, "preserve configured exit order")
	ids[0] = 30
	require.Equal(t, []int64{20, 10}, configured)
}

func TestResolvePool_InvalidConfigurationRemainsError(t *testing.T) {
	for _, extra := range []map[string]any{
		{"egress_proxy_ids": "invalid"},
		{"egress_proxy_ids": []int64{-10}},
		{"egress_proxy_ids": []int64{10}, "egress_include_local": "invalid"},
		{"egress_proxy_ids": []int64{}, "egress_include_local": false},
	} {
		_, _, _, err := ResolvePool(extra, nil, map[int64]string{})
		require.Error(t, err, "malformed pools must not become implicit local exits")
	}
}
