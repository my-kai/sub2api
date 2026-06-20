package store

import (
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/custom/activity/types"
	"github.com/stretchr/testify/require"
)

func TestStoreGetActivityMapsNoRowsToNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	activityStore, err := NewStore(db, "x_")
	require.NoError(t, err)

	mock.ExpectQuery(regexp.QuoteMeta("FROM x_custom_activities")).
		WithArgs(int64(42)).
		WillReturnError(sql.ErrNoRows)

	_, err = activityStore.GetActivity(t.Context(), 42)
	require.True(t, errors.Is(err, types.ErrNotFound))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreClaimSummaryReturnsAllTotals(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	activityStore, err := NewStore(db, "")
	require.NoError(t, err)

	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_red_packet_rain_claims")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"issued", "participants"}).AddRow("12.50000000", int64(3)))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE activity_id = $1 AND round_id = $2 AND user_id = $3")).
		WithArgs(int64(7), int64(9), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"user_round"}).AddRow("2.50000000"))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE activity_id = $1 AND user_id = $2")).
		WithArgs(int64(7), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"user_activity"}).AddRow("6.25000000"))

	summary, err := activityStore.ClaimSummary(t.Context(), 7, 9, 11)
	require.NoError(t, err)
	require.Equal(t, "12.50000000", summary.ActivityIssuedAmount)
	require.Equal(t, "2.50000000", summary.UserRoundAmount)
	require.Equal(t, "6.25000000", summary.UserActivityAmount)
	require.Equal(t, int64(3), summary.ParticipantCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreSettleClaimReturnsDuplicateWithoutCrediting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	activityStore, err := NewStore(db, "")
	require.NoError(t, err)

	createdAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_red_packet_rain_claims")).
		WithArgs(int64(7), int64(9), int64(11), "same-key").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "activity_id", "round_id", "user_id", "hit_count", "reward_amount", "idempotency_key", "created_at",
		}).AddRow(int64(100), int64(7), int64(9), int64(11), 3, "1.25000000", "same-key", createdAt))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_red_packet_rain_claims")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"issued", "participants"}).AddRow("1.25000000", int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE activity_id = $1 AND round_id = $2 AND user_id = $3")).
		WithArgs(int64(7), int64(9), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"user_round"}).AddRow("1.25000000"))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE activity_id = $1 AND user_id = $2")).
		WithArgs(int64(7), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"user_activity"}).AddRow("1.25000000"))
	mock.ExpectCommit()

	result, err := activityStore.SettleClaim(t.Context(), ClaimTransactionInput{
		ActivityID:     7,
		RoundID:        9,
		UserID:         11,
		HitCount:       3,
		IdempotencyKey: "same-key",
	}, func(types.ClaimSummary) (ClaimRewardDecision, error) {
		t.Fatal("duplicate claims must not recalculate reward")
		return ClaimRewardDecision{}, nil
	})
	require.NoError(t, err)
	require.True(t, result.Duplicate)
	require.Equal(t, int64(100), result.Claim.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreSettleClaimCreditsBalanceAndHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	activityStore, err := NewStore(db, "")
	require.NoError(t, err)

	createdAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_xact_lock($1)")).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_red_packet_rain_claims")).
		WithArgs(int64(7), int64(9), int64(11), "new-key").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_red_packet_rain_claims")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"issued", "participants"}).AddRow("0.00000000", int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE activity_id = $1 AND round_id = $2 AND user_id = $3")).
		WithArgs(int64(7), int64(9), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"user_round"}).AddRow("0.00000000"))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE activity_id = $1 AND user_id = $2")).
		WithArgs(int64(7), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"user_activity"}).AddRow("0.00000000"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO custom_red_packet_rain_claims")).
		WithArgs(int64(7), int64(9), int64(11), 4, "2.00000000", "new-key", createdAt).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "activity_id", "round_id", "user_id", "hit_count", "reward_amount", "idempotency_key", "created_at",
		}).AddRow(int64(101), int64(7), int64(9), int64(11), 4, "2.00000000", "new-key", createdAt))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE users")).
		WithArgs("2.00000000", int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO redeem_codes")).
		WithArgs(sqlmock.AnyArg(), activityRewardRedeemType, "2.00000000", "used", int64(11), createdAt, "红包雨奖励：夏日红包雨").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM custom_red_packet_rain_claims")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"issued", "participants"}).AddRow("2.00000000", int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE activity_id = $1 AND round_id = $2 AND user_id = $3")).
		WithArgs(int64(7), int64(9), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"user_round"}).AddRow("2.00000000"))
	mock.ExpectQuery(regexp.QuoteMeta("WHERE activity_id = $1 AND user_id = $2")).
		WithArgs(int64(7), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"user_activity"}).AddRow("2.00000000"))
	mock.ExpectCommit()

	result, err := activityStore.SettleClaim(t.Context(), ClaimTransactionInput{
		ActivityID:     7,
		RoundID:        9,
		UserID:         11,
		HitCount:       4,
		IdempotencyKey: "new-key",
		ActivityTitle:  "夏日红包雨",
		CreatedAt:      createdAt,
	}, func(summary types.ClaimSummary) (ClaimRewardDecision, error) {
		require.Equal(t, "0.00000000", summary.ActivityIssuedAmount)
		return ClaimRewardDecision{RewardAmount: "2.00000000", CreditUserBalance: true}, nil
	})
	require.NoError(t, err)
	require.False(t, result.Duplicate)
	require.Equal(t, "2.00000000", result.Claim.RewardAmount)
	require.Equal(t, "2.00000000", result.Summary.UserActivityAmount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreConsumeWSTicketRejectsReusedOrExpiredTicket(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	activityStore, err := NewStore(db, "")
	require.NoError(t, err)

	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("UPDATE custom_red_packet_rain_ws_tickets")).
		WithArgs("ticket-hash", now).
		WillReturnError(sql.ErrNoRows)

	_, err = activityStore.ConsumeWSTicket(t.Context(), "ticket-hash", now)
	require.True(t, errors.Is(err, types.ErrRedPacketRainSecurityRejected))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStoreMarkWSSessionNonceUsedRejectsReplay(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	activityStore, err := NewStore(db, "")
	require.NoError(t, err)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE custom_red_packet_rain_ws_sessions")).
		WithArgs("session-1", "nonce-1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = activityStore.MarkWSSessionNonceUsed(t.Context(), "session-1", "nonce-1")
	require.True(t, errors.Is(err, types.ErrRedPacketRainSecurityRejected))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScanActivityKeepsUTCAndNullableFields(t *testing.T) {
	createdAt := time.Date(2026, 6, 18, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	endedAt := createdAt.Add(time.Hour)
	activity, err := scanActivity(scanRow{
		int64(1), types.ActivityTypeRedPacketRain, "红包雨", "", "", types.ActivityStatusEnded,
		createdAt, endedAt, int64(2), createdAt, endedAt, endedAt,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), activity.CreatedBy)
	require.NotNil(t, activity.EndedAt)
	require.Equal(t, time.UTC, activity.CreatedAt.Location())
	require.Equal(t, time.UTC, activity.EndedAt.Location())
}

type scanRow []any

func (r scanRow) Scan(dest ...any) error {
	for i := range dest {
		switch target := dest[i].(type) {
		case *int64:
			*target = r[i].(int64)
		case *string:
			*target = r[i].(string)
		case *types.ActivityType:
			*target = r[i].(types.ActivityType)
		case *types.ActivityStatus:
			*target = r[i].(types.ActivityStatus)
		case *time.Time:
			*target = r[i].(time.Time)
		case *sql.NullInt64:
			*target = sql.NullInt64{Int64: r[i].(int64), Valid: true}
		case *sql.NullTime:
			*target = sql.NullTime{Time: r[i].(time.Time), Valid: true}
		default:
			return errors.New("unsupported scan target")
		}
	}
	return nil
}
