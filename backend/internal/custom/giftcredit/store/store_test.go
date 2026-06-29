package store

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/custom/giftcredit/types"
	"github.com/stretchr/testify/require"
)

func TestCreateGrantInsertsGrantAndAggregate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	giftStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(24 * time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO custom_gift_credit_grants")).
		WithArgs(int64(7), types.SourceAdminGrant, "manual-1", "1.25000000", expiresAt, types.StatusActive, "note", sqlmock.AnyArg(), now).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "source_type", "source_id", "original_amount", "remaining_amount",
			"expires_at", "status", "note", "created_by", "created_at", "updated_at",
		}).AddRow(int64(11), int64(7), types.SourceAdminGrant, "manual-1", "1.25000000", "1.25000000",
			expiresAt, types.StatusActive, "note", nil, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO custom_gift_credit_user_balances")).
		WithArgs(int64(7), "1.25000000", expiresAt, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	grant, err := giftStore.CreateGrant(context.Background(), types.CreateGrantInput{
		UserID:     7,
		SourceType: types.SourceAdminGrant,
		SourceID:   "manual-1",
		Amount:     "1.25",
		ExpiresAt:  &expiresAt,
		Note:       "note",
		CreatedAt:  now,
	})
	require.NoError(t, err)
	require.Equal(t, "1.25000000", grant.OriginalAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateGrantAllowsPermanentExpiry(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	giftStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO custom_gift_credit_grants")).
		WithArgs(int64(7), types.SourceAdminGrant, "manual-permanent", "1.25000000", nil, types.StatusActive, "note", sqlmock.AnyArg(), now).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "source_type", "source_id", "original_amount", "remaining_amount",
			"expires_at", "status", "note", "created_by", "created_at", "updated_at",
		}).AddRow(int64(11), int64(7), types.SourceAdminGrant, "manual-permanent", "1.25000000", "1.25000000",
			nil, types.StatusActive, "note", nil, now, now))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO custom_gift_credit_user_balances")).
		WithArgs(int64(7), "1.25000000", nil, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	grant, err := giftStore.CreateGrant(context.Background(), types.CreateGrantInput{
		UserID:     7,
		SourceType: types.SourceAdminGrant,
		SourceID:   "manual-permanent",
		Amount:     "1.25",
		Note:       "note",
		CreatedAt:  now,
	})
	require.NoError(t, err)
	require.Nil(t, grant.ExpiresAt)
	require.Equal(t, "1.25000000", grant.OriginalAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateGrantRequiresSourceID(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	giftStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(24 * time.Hour)
	_, err = giftStore.CreateGrant(context.Background(), types.CreateGrantInput{
		UserID:     7,
		SourceType: types.SourceAdminGrant,
		Amount:     "1.25",
		ExpiresAt:  &expiresAt,
		CreatedAt:  now,
	})

	require.ErrorIs(t, err, types.ErrInvalidInput)
}

func TestDeductFirstSkipsGrantLookupWhenAggregateZero(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	giftStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "0.00000000", nil, now, now))

	result, err := giftStore.DeductFirst(context.Background(), db, types.DeductInput{
		UserID:    9,
		Amount:    "2.5",
		RequestID: "req-1",
		Now:       now,
	})
	require.NoError(t, err)
	require.True(t, result.SkippedGrantLookup)
	require.Equal(t, "0.00000000", result.GiftDeducted)
	require.Equal(t, "2.50000000", result.RemainingCost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductFirstRefreshesExpiredAggregateBeforeDeduct(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	giftStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "2.00000000", expiredAt, expiredAt, expiredAt))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_gift_credit_grants")).
		WithArgs(types.StatusExpired, now, int64(9), types.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO custom_gift_credit_user_balances")).
		WithArgs(int64(9), now, types.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "0.00000000", nil, now, now))

	result, err := giftStore.DeductFirst(context.Background(), db, types.DeductInput{
		UserID:    9,
		Amount:    "1.25",
		RequestID: "req-expired",
		Now:       now,
	})
	require.NoError(t, err)
	require.True(t, result.SkippedGrantLookup)
	require.Equal(t, "0.00000000", result.GiftDeducted)
	require.Equal(t, "1.25000000", result.RemainingCost)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductFirstConsumesOldestGrant(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	giftStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "3.00000000", expiresAt, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, source_type, source_id, original_amount::text, remaining_amount::text")).
		WithArgs(int64(9), types.StatusActive, now, giftCreditDeductBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "source_type", "source_id", "original_amount", "remaining_amount",
			"expires_at", "status", "note", "created_by", "created_at", "updated_at",
		}).AddRow(int64(1), int64(9), types.SourcePromoCode, "promo:1", "3.00000000", "3.00000000",
			expiresAt, types.StatusActive, "", nil, now, now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_gift_credit_grants")).
		WithArgs("1.75000000", types.StatusActive, now, int64(1), int64(9), "1.25000000", types.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO custom_gift_credit_deductions")).
		WithArgs(int64(1), int64(9), "req-1", "ub-1", "1.25000000", now).
		WillReturnRows(sqlmock.NewRows([]string{"id", "grant_id", "user_id", "request_id", "usage_billing_key", "amount", "created_at"}).
			AddRow(int64(5), int64(1), int64(9), "req-1", "ub-1", "1.25000000", now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_gift_credit_user_balances")).
		WithArgs(int64(9), "1.75000000", types.StatusActive, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "1.75000000", expiresAt, now, now))

	result, err := giftStore.DeductFirst(context.Background(), db, types.DeductInput{
		UserID:          9,
		Amount:          "1.25",
		RequestID:       "req-1",
		UsageBillingKey: "ub-1",
		Now:             now,
	})
	require.NoError(t, err)
	require.False(t, result.SkippedGrantLookup)
	require.Equal(t, "1.25000000", result.GiftDeducted)
	require.Equal(t, "0.00000000", result.RemainingCost)
	require.Equal(t, "1.75000000", result.NewGiftBalance)
	require.Len(t, result.Deductions, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductFirstConsumesPermanentGrant(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	giftStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "3.00000000", nil, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, source_type, source_id, original_amount::text, remaining_amount::text")).
		WithArgs(int64(9), types.StatusActive, now, giftCreditDeductBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "source_type", "source_id", "original_amount", "remaining_amount",
			"expires_at", "status", "note", "created_by", "created_at", "updated_at",
		}).AddRow(int64(1), int64(9), types.SourcePromoCode, "promo:permanent", "3.00000000", "3.00000000",
			nil, types.StatusActive, "", nil, now, now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_gift_credit_grants")).
		WithArgs("1.75000000", types.StatusActive, now, int64(1), int64(9), "1.25000000", types.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO custom_gift_credit_deductions")).
		WithArgs(int64(1), int64(9), "req-permanent", "ub-permanent", "1.25000000", now).
		WillReturnRows(sqlmock.NewRows([]string{"id", "grant_id", "user_id", "request_id", "usage_billing_key", "amount", "created_at"}).
			AddRow(int64(5), int64(1), int64(9), "req-permanent", "ub-permanent", "1.25000000", now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_gift_credit_user_balances")).
		WithArgs(int64(9), "1.75000000", types.StatusActive, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "1.75000000", nil, now, now))

	result, err := giftStore.DeductFirst(context.Background(), db, types.DeductInput{
		UserID:          9,
		Amount:          "1.25",
		RequestID:       "req-permanent",
		UsageBillingKey: "ub-permanent",
		Now:             now,
	})
	require.NoError(t, err)
	require.False(t, result.SkippedGrantLookup)
	require.Equal(t, "1.25000000", result.GiftDeducted)
	require.Equal(t, "0.00000000", result.RemainingCost)
	require.Equal(t, "1.75000000", result.NewGiftBalance)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeductFirstDepletesPartialGiftCreditAndReturnsRemainingCost(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	giftStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "0.75000000", expiresAt, now, now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, user_id, source_type, source_id, original_amount::text, remaining_amount::text")).
		WithArgs(int64(9), types.StatusActive, now, giftCreditDeductBatchSize).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "source_type", "source_id", "original_amount", "remaining_amount",
			"expires_at", "status", "note", "created_by", "created_at", "updated_at",
		}).AddRow(int64(1), int64(9), types.SourcePromoCode, "promo:1", "0.75000000", "0.75000000",
			expiresAt, types.StatusActive, "", nil, now, now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_gift_credit_grants")).
		WithArgs("0.00000000", types.StatusDepleted, now, int64(1), int64(9), "0.75000000", types.StatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO custom_gift_credit_deductions")).
		WithArgs(int64(1), int64(9), "req-1", "ub-1", "0.75000000", now).
		WillReturnRows(sqlmock.NewRows([]string{"id", "grant_id", "user_id", "request_id", "usage_billing_key", "amount", "created_at"}).
			AddRow(int64(5), int64(1), int64(9), "req-1", "ub-1", "0.75000000", now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_gift_credit_user_balances")).
		WithArgs(int64(9), "0.00000000", types.StatusActive, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id, active_remaining_amount::text, next_expires_at, refreshed_at, updated_at")).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "active_remaining_amount", "next_expires_at", "refreshed_at", "updated_at"}).
			AddRow(int64(9), "0.00000000", nil, now, now))

	result, err := giftStore.DeductFirst(context.Background(), db, types.DeductInput{
		UserID:          9,
		Amount:          "1.25",
		RequestID:       "req-1",
		UsageBillingKey: "ub-1",
		Now:             now,
	})
	require.NoError(t, err)
	require.False(t, result.SkippedGrantLookup)
	require.Equal(t, "0.75000000", result.GiftDeducted)
	require.Equal(t, "0.50000000", result.RemainingCost)
	require.Equal(t, "0.00000000", result.NewGiftBalance)
	require.Len(t, result.Deductions, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}
