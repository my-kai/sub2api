package callbackauth

import (
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestStoreCreateAndConsumeCode(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	fixedNow := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	store, err := NewStore(db, 5*time.Minute)
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow }

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO custom_callback_auth_codes")).
		WithArgs(sqlmock.AnyArg(), int64(7), "https://client.example/cb", "client.example", fixedNow.Add(5*time.Minute), fixedNow).
		WillReturnResult(sqlmock.NewResult(1, 1))

	code, created, err := store.CreateCode(t.Context(), 7, "https://client.example/cb", "client.example")
	require.NoError(t, err)
	require.NotEmpty(t, code)
	require.Equal(t, int64(7), created.UserID)
	require.Equal(t, fixedNow.Add(5*time.Minute), created.ExpiresAt)

	rows := sqlmock.NewRows([]string{"user_id", "callback_url", "callback_domain", "expires_at", "created_at"}).
		AddRow(int64(7), "https://client.example/cb", "client.example", created.ExpiresAt, created.CreatedAt)
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_callback_auth_codes")).
		WithArgs(hashCode(code), fixedNow).
		WillReturnRows(rows)

	consumed, err := store.ConsumeCode(t.Context(), code)
	require.NoError(t, err)
	require.Equal(t, created, consumed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreConsumeCodeExpired(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	fixedNow := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	store, err := NewStore(db, time.Minute)
	require.NoError(t, err)
	store.now = func() time.Time { return fixedNow }

	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_callback_auth_codes")).
		WithArgs(hashCode("missing"), fixedNow).
		WillReturnError(sql.ErrNoRows)

	_, err = store.ConsumeCode(t.Context(), "missing")
	require.True(t, errors.Is(err, ErrCodeExpired))
	require.NoError(t, mock.ExpectationsWereMet())
}
