package pgstorage_test

import (
	"database/sql"
	"github.com/adamluzsi/frameless/resources/storages/pgstorage"
	"github.com/adamluzsi/frameless/queries"
	"os"
	"testing"
)

func ExamplePG(tb testing.TB) *pgstorage.PG {
	storage, err := pgstorage.NewPG(DatabaseURL(tb))

	if err != nil {
		tb.Fatal(err)
	}

	return storage
}

func TestPG(t *testing.T) {
	t.Skip()

	if _, ok := os.LookupEnv("NO_DB"); ok {
		t.Skip()
	}

	subject := ExamplePG(t)
	migrateDB(t, subject.DB)
	queries.TestAll(t, subject, resetDBFunc(t, subject.DB))
}

const CreateTemporaryTableQuery = `
CREATE TEMPORARY TABLE $1 (
	id SERIAL,
	data varchar
);
`

func migrateDB(t *testing.T, db *sql.DB) {

	if _, err := db.Exec(CreateTemporaryTableQuery, `exported_entities`); err != nil {
		t.Fatal(err)
		return
	}

	if _, err := db.Exec(CreateTemporaryTableQuery, `unexported_entities`); err != nil {
		t.Fatal(err)
		return
	}

}

func resetDBFunc(t *testing.T, db *sql.DB) func() {
	return func() {

		if _, err := db.Exec(`DROP TABLE $1`, `exported_entities`); err != nil {
			t.Fatal(err)
		}

		if _, err := db.Exec(`DROP TABLE $1`, `unexported_entities`); err != nil {
			t.Fatal(err)
		}

		migrateDB(t, db)

	}
}

func DatabaseURL(tb testing.TB) string {
	value, ok := os.LookupEnv("PG_DATABASE_URL")

	if !ok {
		tb.Fatal("PG_DATABASE_URL not set in the local environment, cannot proceed with testing")
	}

	return value
}
