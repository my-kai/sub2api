package accountdefaults

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newStoreMock(t *testing.T) (*Store, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	store, err := NewStore(db)
	require.NoError(t, err)
	return store, mock, func() {
		mock.ExpectClose()
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	}
}

// 缺行 / 空对象 / 脏 JSON 都必须退化为零值配置而不是报错：
// 建号表单的默认带入只是便捷能力，读取失败不能阻断管理员操作。
func TestLoadConfigDegradation(t *testing.T) {
	t.Run("missing row returns zero config", func(t *testing.T) {
		store, mock, cleanup := newStoreMock(t)
		defer cleanup()
		mock.ExpectQuery(`SELECT config FROM custom_account_defaults`).
			WillReturnRows(sqlmock.NewRows([]string{"config"}))

		cfg, err := store.loadConfig(context.Background())
		require.NoError(t, err)
		require.Equal(t, Config{ModelsByPlatform: map[string][]string{}, ProxyFixedIDs: []int64{}}, cfg)
	})

	t.Run("unparsable payload returns zero config", func(t *testing.T) {
		store, mock, cleanup := newStoreMock(t)
		defer cleanup()
		mock.ExpectQuery(`SELECT config FROM custom_account_defaults`).
			WillReturnRows(sqlmock.NewRows([]string{"config"}).AddRow([]byte(`[1,2,3]`)))

		cfg, err := store.loadConfig(context.Background())
		require.NoError(t, err)
		require.Empty(t, cfg.ModelsByPlatform)
		require.Empty(t, cfg.ProxyFixedIDs)
	})
}

// 归一化必须去重、去空串、平台 key 转小写，且不能与调用方共享底层数组。
func TestNormalizeConfigCleansInput(t *testing.T) {
	inputModels := []string{"glm-5.1", " glm-5.1 ", "", "kimi-k3"}
	cfg := normalizeConfig(Config{
		ModelsByPlatform: map[string][]string{"  OpenAI ": inputModels},
		ProxyMode:        ProxyModeFixed,
		ProxyFixedIDs:    []int64{0, 3, 3, -1, 5},
	})

	require.Equal(t, []string{"glm-5.1", "kimi-k3"}, cfg.ModelsByPlatform["openai"])
	require.Equal(t, []int64{3, 5}, cfg.ProxyFixedIDs)

	// 调用方后续修改输入不应污染已归一化的配置。
	inputModels[0] = "mutated"
	require.Equal(t, "glm-5.1", cfg.ModelsByPlatform["openai"][0])

	// 未知 mode 归一化为空（= 不启用代理默认带入）。
	require.Equal(t, ProxyMode(""), normalizeConfig(Config{ProxyMode: "pool"}).ProxyMode)
}

// resolve 是表单带入的核心语义：random 抽 1、fixed 按配置顺序过滤失效代理。
func TestResolveProxyDefaults(t *testing.T) {
	newService := func(t *testing.T, rows [][]any, proxies []service.Proxy, byIDs map[int64]service.Proxy) (*Service, *fakeProxyPool) {
		store, mock, cleanup := newStoreMock(t)
		t.Cleanup(cleanup)
		cfgJSON := rows[0][0].(string)
		mock.ExpectQuery(`SELECT config FROM custom_account_defaults`).
			WillReturnRows(sqlmock.NewRows([]string{"config"}).AddRow([]byte(cfgJSON)))
		pool := &fakeProxyPool{available: proxies, byID: byIDs}
		svc, err := NewService(store, pool)
		require.NoError(t, err)
		return svc, pool
	}

	t.Run("random mode picks exactly one available proxy", func(t *testing.T) {
		proxies := []service.Proxy{
			{ID: 11, Name: "a", Host: "a.example", Status: service.StatusActive},
			{ID: 22, Name: "b", Host: "b.example", Status: service.StatusActive},
		}
		svc, _ := newService(t, [][]any{{
			`{"proxy_mode":"random","models_by_platform":{"openai":["gpt-5.6"]}}`,
		}}, proxies, nil)

		resolved, err := svc.resolve(context.Background())
		require.NoError(t, err)
		require.Len(t, resolved.EgressProxyIDs, 1)
		require.Contains(t, []int64{11, 22}, resolved.EgressProxyIDs[0])
		require.Equal(t, []string{"gpt-5.6"}, resolved.ModelsByPlatform["openai"])
	})

	t.Run("random mode with empty pool keeps form empty", func(t *testing.T) {
		svc, _ := newService(t, [][]any{{`{"proxy_mode":"random"}`}}, nil, nil)
		resolved, err := svc.resolve(context.Background())
		require.NoError(t, err)
		require.Empty(t, resolved.EgressProxyIDs)
	})

	t.Run("fixed mode keeps config order and drops unavailable proxies", func(t *testing.T) {
		svc, _ := newService(t, [][]any{{
			`{"proxy_mode":"fixed","proxy_fixed_ids":[30,10,20],"allow_local_egress":true}`,
		}}, nil, map[int64]service.Proxy{
			10: {ID: 10, Host: "h10", Status: service.StatusActive},
			20: {ID: 20, Host: "h20", Status: service.StatusExpired},
			30: {ID: 30, Host: "h30", Status: service.StatusActive},
		})

		resolved, err := svc.resolve(context.Background())
		require.NoError(t, err)
		require.Equal(t, []int64{30, 10}, resolved.EgressProxyIDs)
		require.True(t, resolved.EgressIncludeLocal)
	})

	t.Run("no proxy mode leaves form untouched", func(t *testing.T) {
		svc, _ := newService(t, [][]any{{`{}`}}, nil, nil)
		resolved, err := svc.resolve(context.Background())
		require.NoError(t, err)
		require.Empty(t, resolved.EgressProxyIDs)
		require.Empty(t, resolved.ProxyMode)
	})
}

// 固定模式必须至少有一个出口，否则表单必然带入空出口，保存时应拒绝。
func TestSaveConfigValidation(t *testing.T) {
	store, mock, cleanup := newStoreMock(t)
	defer cleanup()
	svc, err := NewService(store, &fakeProxyPool{})
	require.NoError(t, err)

	_, err = svc.SaveConfig(context.Background(), Config{ProxyMode: ProxyModeFixed}, 7)
	require.ErrorIs(t, err, ErrInvalidConfig)

	_, err = svc.SaveConfig(context.Background(), Config{ProxyMode: "pool"}, 7)
	require.ErrorIs(t, err, ErrInvalidConfig)

	mock.ExpectQuery(`UPDATE custom_account_defaults`).
		WillReturnRows(sqlmock.NewRows([]string{"config"}).
			AddRow([]byte(`{"proxy_mode":"fixed","proxy_fixed_ids":[9],"models_by_platform":{},"allow_local_egress":false}`)))
	saved, err := svc.SaveConfig(context.Background(), Config{ProxyMode: ProxyModeFixed, ProxyFixedIDs: []int64{9}}, 7)
	require.NoError(t, err)
	require.Equal(t, []int64{9}, saved.ProxyFixedIDs)
}

type fakeProxyPool struct {
	available []service.Proxy
	byID      map[int64]service.Proxy
}

func (f *fakeProxyPool) ListActive(context.Context) ([]service.Proxy, error) {
	return append([]service.Proxy(nil), f.available...), nil
}

func (f *fakeProxyPool) ListByIDs(_ context.Context, ids []int64) ([]service.Proxy, error) {
	out := make([]service.Proxy, 0, len(ids))
	for _, id := range ids {
		if p, ok := f.byID[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// 结构化断言：fake 必须满足窄接口（service.ProxyRepository 亦满足）。
var _ ProxyPool = (*fakeProxyPool)(nil)
