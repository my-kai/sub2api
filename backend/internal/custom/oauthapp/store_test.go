package oauthapp

import (
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateAndConsumeCodeWithValue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	fixedNow := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	store, err := NewStore(db, 5*time.Minute)
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow }

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO custom_oauth_authorization_codes")).
		WithArgs(hashCode("code-1"), int64(7), "ak_test", "https://app.example.com/cb", "app.example.com", fixedNow.Add(5*time.Minute), fixedNow).
		WillReturnResult(sqlmock.NewResult(1, 1))

	created, err := store.CreateCodeWithValue(t.Context(), "code-1", 7, "ak_test", "https://app.example.com/cb", "app.example.com")
	require.NoError(t, err)
	require.Equal(t, int64(7), created.UserID)
	require.Equal(t, "ak_test", created.AccessKey)
	require.Equal(t, fixedNow.Add(5*time.Minute), created.ExpiresAt)

	rows := sqlmock.NewRows([]string{"user_id", "access_key", "redirect_uri", "redirect_domain", "expires_at", "created_at"}).
		AddRow(int64(7), "ak_test", "https://app.example.com/cb", "app.example.com", created.ExpiresAt, created.CreatedAt)
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_oauth_authorization_codes")).
		WithArgs(hashCode("code-1"), fixedNow).
		WillReturnRows(rows)

	consumed, err := store.ConsumeCode(t.Context(), "code-1")
	require.NoError(t, err)
	require.Equal(t, created, consumed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreConsumeCodeExpired(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	fixedNow := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	store, err := NewStore(db, time.Minute)
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow }

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_oauth_authorization_codes")).
		WithArgs(hashCode("missing"), fixedNow).
		WillReturnError(sql.ErrNoRows)

	_, err = store.ConsumeCode(t.Context(), "missing")
	require.True(t, errors.Is(err, ErrCodeExpired))
	require.NoError(t, mock.ExpectationsWereMet())
}
