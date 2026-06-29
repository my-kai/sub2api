package runtime

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestValidateTablePrefix(t *testing.T) {
	require.NoError(t, ValidateTablePrefix(""))
	require.NoError(t, ValidateTablePrefix("tenant1_"))
	require.Error(t, ValidateTablePrefix("tenant-1"))
}

func TestApplyMigrationsFSRejectsChecksumMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	fsys := fstest.MapFS{
		"001_test.sql": {Data: []byte("CREATE TABLE {{TABLE_PREFIX}}gift_test (id BIGINT);")},
	}
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS custom_gift_credit_schema_migrations").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT checksum FROM custom_gift_credit_schema_migrations WHERE filename =").
		WithArgs("001_test.sql").
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow("different"))

	err = ApplyMigrationsFS(context.Background(), db, fsys, MigrationOptions{})
	require.ErrorContains(t, err, "checksum mismatch")
	require.NoError(t, mock.ExpectationsWereMet())
}
