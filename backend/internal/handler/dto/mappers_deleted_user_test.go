package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserFromServiceShallow_MapsDeletedAt(t *testing.T) {
	ts := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)

	deleted := UserFromServiceShallow(&service.User{ID: 1, Email: "d@test.com", DeletedAt: &ts})
	require.NotNil(t, deleted.DeletedAt)
	require.Equal(t, ts, *deleted.DeletedAt)

	active := UserFromServiceShallow(&service.User{ID: 2, Email: "a@test.com"})
	require.Nil(t, active.DeletedAt, "active user must have nil DeletedAt")
}

func TestUserFromServiceShallow_UsesHydratedAvailableBalance(t *testing.T) {
	user := UserFromServiceShallow(&service.User{
		ID:               3,
		Email:            "balance@test.com",
		Balance:          1.11111111,
		GiftBalance:      2.22222222,
		AvailableBalance: 3.33333333,
	})

	require.Equal(t, 1.11111111, user.Balance)
	require.Equal(t, 2.22222222, user.GiftBalance)
	require.Equal(t, 3.33333333, user.AvailableBalance)
}

func TestUserFromServiceShallow_RequiresHydratedAvailableBalance(t *testing.T) {
	user := UserFromServiceShallow(&service.User{
		ID:          4,
		Email:       "explicit-balance@test.com",
		Balance:     1.11111111,
		GiftBalance: 2.22222222,
	})

	require.Zero(t, user.AvailableBalance)
}
