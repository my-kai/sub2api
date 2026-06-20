package runtime

import (
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestApplyMigrationsFSUsesCustomActivityMigrationTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS custom_activity_schema_migrations`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT checksum FROM custom_activity_schema_migrations WHERE filename = \$1`).
		WithArgs("001.sql").
		WillReturnError(sqlmock.ErrCancelled)

	err = ApplyMigrationsFS(t.Context(), db, fstest.MapFS{
		"001.sql": {Data: []byte("CREATE TABLE {{TABLE_PREFIX}}custom_activities (id BIGINT);")},
	}, MigrationOptions{TablePrefix: "x_"})
	if err == nil {
		t.Fatalf("ApplyMigrationsFS() should surface query errors")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestSafeIdentifierRejectsUnsafeActivityMigrationTableName(t *testing.T) {
	if _, err := safeIdentifier("custom_activity_schema_migrations;drop"); err == nil {
		t.Fatalf("safeIdentifier() should reject unsafe names")
	}
}
