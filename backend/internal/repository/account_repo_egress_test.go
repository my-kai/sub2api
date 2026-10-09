package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func newEgressTestAccountRepo(t *testing.T) (*accountRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	return newAccountRepositoryWithSQL(client, nil, nil), mock
}

func egressTestProxyRows() *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{"id", "name", "host", "status", "deleted_at", "expires_at"}).
		AddRow(int64(10), "disabled", "disabled.example", "inactive", nil, nil).
		AddRow(int64(20), "healthy", "healthy.example", service.StatusActive, nil, nil).
		AddRow(int64(30), "expired", "expired.example", service.StatusActive, nil, now.Add(-time.Hour)).
		AddRow(int64(40), "deleted", "deleted.example", service.StatusActive, now, nil)
}

func egressTestEntity(id int64, ids []int64, includeLocal bool) *dbent.Account {
	return &dbent.Account{
		ID: id, Name: "egress-test", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true,
		Extra: map[string]any{"egress_proxy_ids": ids, "egress_include_local": includeLocal},
	}
}

func TestAccountsToService_EgressIsolation(t *testing.T) {
	repo, mock := newEgressTestAccountRepo(t)
	inactiveID := int64(10)
	accounts := []*dbent.Account{
		egressTestEntity(1, []int64{10, 20, 30, 40, 50}, false),
		egressTestEntity(2, []int64{10, 30, 40, 50}, false),
		egressTestEntity(3, []int64{10, 50}, true),
		{ID: 4, Status: service.StatusActive, Schedulable: true, ProxyID: &inactiveID},
		// The fallback origin is display-only. Its inactive state must not
		// invalidate the account's currently configured direct exit.
		{ID: 5, Status: service.StatusActive, Schedulable: true, ProxyFallbackOriginID: &inactiveID},
		egressTestEntity(6, []int64{20}, false),
	}
	accounts[5].Schedulable = false
	mock.ExpectQuery(`FROM "proxies"`).WillReturnRows(egressTestProxyRows())
	mock.ExpectQuery(`FROM "account_groups"`).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))

	got, err := repo.accountsToService(context.Background(), accounts)
	require.NoError(t, err, "one unavailable exit must not abort the entire scheduler bucket")
	require.Len(t, got, len(accounts))
	require.True(t, got[0].IsSchedulable())
	require.Equal(t, []int64{10, 20, 30, 40, 50}, got[0].EgressProxyIDs, "retain configured IDs for repair")
	require.Len(t, got[0].EgressProxies, 1)
	require.Equal(t, int64(20), got[0].EgressProxies[0].ID)
	require.Equal(t, "healthy.example", got[0].SelectEgressForRequest().SelectedEgressHost)
	for _, index := range []int{1, 3} {
		require.False(t, got[index].IsSchedulable())
		require.True(t, got[index].HasConfiguredEgress())
		require.Nil(t, got[index].SelectEgressForRequest(), "unavailable exits must never become implicit direct traffic")
	}
	require.Equal(t, &inactiveID, got[3].ProxyID)
	require.Equal(t, []int64{10}, got[3].EgressProxyIDs)
	require.Nil(t, got[3].Proxy)
	for _, index := range []int{2, 4} {
		require.True(t, got[index].IsSchedulable())
		require.Equal(t, "local", got[index].SelectEgressForRequest().SelectedEgressKey)
	}
	require.False(t, got[5].Schedulable, "a usable exit must not re-enable an already disabled account")
	require.True(t, accounts[1].Schedulable, "hydration must not persist the runtime egress decision")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountRuntimeReads_EgressIsolation(t *testing.T) {
	for _, method := range []string{"single", "batch", "platform", "group"} {
		t.Run(method, func(t *testing.T) {
			repo, mock := newEgressTestAccountRepo(t)
			if method == "group" {
				mock.ExpectQuery(`FROM "account_groups"`).WillReturnRows(
					sqlmock.NewRows([]string{"id", "account_id", "group_id", "priority"}).AddRow(int64(1), int64(1), int64(32), 1))
			}
			// Explicit pools exercise the same read that failed for group 32.
			mock.ExpectQuery(`FROM "accounts"`).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "schedulable", "platform", "extra"}).
				AddRow(int64(1), service.StatusActive, true, service.PlatformOpenAI, `{"egress_proxy_ids":[10,20,30,40,50],"egress_include_local":false}`))
			// GetByIDs has a separately optimized hydration path: verify that its
			// group/proxy query ordering still applies the identical exit policy.
			if method == "batch" {
				mock.ExpectQuery(`FROM "account_groups"`).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
			}
			mock.ExpectQuery(`FROM "proxies"`).WillReturnRows(egressTestProxyRows())
			if method != "batch" {
				mock.ExpectQuery(`FROM "account_groups"`).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
			}

			var got *service.Account
			var err error
			switch method {
			case "single":
				got, err = repo.GetByID(context.Background(), 1)
			case "batch":
				var accounts []*service.Account
				accounts, err = repo.GetByIDs(context.Background(), []int64{1})
				if err == nil {
					require.Len(t, accounts, 1)
					got = accounts[0]
				}
			case "platform", "group":
				var accounts []service.Account
				if method == "group" {
					accounts, err = repo.ListSchedulableByGroupIDAndPlatform(context.Background(), 32, service.PlatformOpenAI)
				} else {
					accounts, err = repo.ListSchedulableByPlatform(context.Background(), service.PlatformOpenAI)
				}
				if err == nil {
					require.Len(t, accounts, 1)
					got = &accounts[0]
				}
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			require.True(t, got.IsSchedulable())
			require.Len(t, got.EgressProxies, 1)
			require.Equal(t, int64(20), got.EgressProxies[0].ID)
			require.Equal(t, "proxy:20", got.SelectEgressForRequest().SelectedEgressKey)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAccountsToService_AdminPreservesInactiveEgress(t *testing.T) {
	repo, mock := newEgressTestAccountRepo(t)
	mock.ExpectQuery(`FROM "proxies"`).WillReturnRows(egressTestProxyRows())
	mock.ExpectQuery(`FROM "account_groups"`).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))

	got, err := repo.accountsToServiceWithProxyPolicy(context.Background(), []*dbent.Account{egressTestEntity(1, []int64{10, 30, 50}, false)}, false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.True(t, got[0].Schedulable, "admin reads must retain the stored account setting")
	require.Equal(t, []int64{10, 30, 50}, got[0].EgressProxyIDs)
	require.Len(t, got[0].EgressProxies, 2)
	require.Equal(t, "disabled（失效）", got[0].EgressProxies[0].Name)
	require.Equal(t, "expired（失效）", got[0].EgressProxies[1].Name)
	require.Nil(t, got[0].SelectEgressForRequest())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAccountsToService_EgressQueryErrorPropagates(t *testing.T) {
	repo, mock := newEgressTestAccountRepo(t)
	storageErr := errors.New("proxy storage unavailable")
	mock.ExpectQuery(`FROM "proxies"`).WillReturnError(storageErr)

	_, err := repo.accountsToService(context.Background(), []*dbent.Account{egressTestEntity(1, []int64{10}, true)})
	require.ErrorIs(t, err, storageErr, "do not turn database failures into successful partial snapshots")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByIDs_LegacyEgressIsolationAndAdminRepair(t *testing.T) {
	for _, requireActive := range []bool{true, false} {
		for _, state := range []string{"inactive", "expired", "missing"} {
			t.Run(fmt.Sprintf("runtime=%t/%s", requireActive, state), func(t *testing.T) {
				repo, mock := newEgressTestAccountRepo(t)
				mock.ExpectQuery(`FROM "accounts"`).WillReturnRows(sqlmock.NewRows([]string{"id", "status", "schedulable", "proxy_id"}).
					AddRow(int64(1), service.StatusActive, true, int64(10)))
				mock.ExpectQuery(`FROM "account_groups"`).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
				rows := sqlmock.NewRows([]string{"id", "name", "host", "status", "expires_at"})
				switch state {
				case "inactive":
					rows.AddRow(int64(10), "stale", "stale.example", service.StatusDisabled, nil)
				case "expired":
					rows.AddRow(int64(10), "stale", "stale.example", service.StatusActive, time.Now().Add(-time.Hour))
				}
				mock.ExpectQuery(`FROM "proxies"`).WillReturnRows(rows)

				var got []*service.Account
				var err error
				if requireActive {
					got, err = repo.GetByIDs(context.Background(), []int64{1})
				} else {
					got, err = repo.GetByIDsIncludingInactiveEgress(context.Background(), []int64{1})
				}
				require.NoError(t, err)
				require.Len(t, got, 1)
				require.Equal(t, int64(10), *got[0].ProxyID)
				require.Equal(t, []int64{10}, got[0].EgressProxyIDs)
				require.False(t, got[0].EgressIncludeLocal)
				require.Equal(t, !requireActive, got[0].Schedulable)
				require.Nil(t, got[0].SelectEgressForRequest())
				if !requireActive && state != "missing" {
					require.Len(t, got[0].EgressProxies, 1)
					require.Equal(t, "stale（失效）", got[0].Proxy.Name)
				} else {
					require.Empty(t, got[0].EgressProxies)
					require.Nil(t, got[0].Proxy)
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

func TestAccountsToService_EgressRecoversAfterProxyReactivation(t *testing.T) {
	repo, mock := newEgressTestAccountRepo(t)
	entity := egressTestEntity(1, []int64{10}, false)
	for _, status := range []string{service.StatusDisabled, service.StatusActive} {
		mock.ExpectQuery(`FROM "proxies"`).WillReturnRows(sqlmock.NewRows([]string{"id", "host", "status"}).
			AddRow(int64(10), "restored.example", status))
		mock.ExpectQuery(`FROM "account_groups"`).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
		got, err := repo.accountsToService(context.Background(), []*dbent.Account{entity})
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, status == service.StatusActive, got[0].IsSchedulable())
		require.True(t, entity.Schedulable, "exit failure must not permanently pause the stored account")
	}
	require.NoError(t, mock.ExpectationsWereMet())
}
