//go:build unit

package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSchedulerSnapshot_EgressIsolationPreservesFailClosedMetadata(t *testing.T) {
	ctx := context.Background()
	repo, mock := newEgressTestAccountRepo(t)
	mock.ExpectQuery(`FROM "proxies"`).WillReturnRows(egressTestProxyRows())
	mock.ExpectQuery(`FROM "account_groups"`).WillReturnRows(sqlmock.NewRows([]string{"account_id"}))
	accounts, err := repo.accountsToService(ctx, []*dbent.Account{
		egressTestEntity(1, []int64{10}, false),
		egressTestEntity(2, []int64{10, 20}, false),
		egressTestEntity(3, []int64{10}, true),
	})
	require.NoError(t, err)
	cache := newSchedulerCacheUnit(t)

	// Single and forced buckets share a query result in a rebuild. Both must
	// preserve the runtime decision in their lightweight metadata, while the
	// full account payload retains IDs for fail-closed request hydration.
	for _, mode := range []string{service.SchedulerModeSingle, service.SchedulerModeForced} {
		bucket := service.SchedulerBucket{GroupID: 32, Platform: service.PlatformOpenAI, Mode: mode}
		token, err := cache.CaptureBucketWriteToken(ctx, bucket)
		require.NoError(t, err)
		require.NoError(t, cache.SetSnapshot(ctx, bucket, token, accounts))
		snapshot, hit, err := cache.GetSnapshot(ctx, bucket)
		require.NoError(t, err)
		require.True(t, hit)
		require.Len(t, snapshot, 3)
		require.False(t, snapshot[0].IsSchedulable(), "metadata must exclude the exitless account from selection")
		require.True(t, snapshot[1].IsSchedulable())
		require.True(t, snapshot[2].IsSchedulable())
	}

	blocked, err := cache.GetAccount(ctx, 1)
	require.NoError(t, err)
	require.False(t, blocked.Schedulable)
	require.Equal(t, []int64{10}, blocked.EgressProxyIDs)
	require.Nil(t, blocked.SelectEgressForRequest(), "full cached account must never fall back to local")
	healthy, err := cache.GetAccount(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, "proxy:20", healthy.SelectEgressForRequest().SelectedEgressKey)
	direct, err := cache.GetAccount(ctx, 3)
	require.NoError(t, err)
	require.Equal(t, "local", direct.SelectEgressForRequest().SelectedEgressKey)
	require.NoError(t, mock.ExpectationsWereMet())
}
