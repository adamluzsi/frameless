package pgstroage_test

import (
	"database/sql"
	"github.com/adamluzsi/frameless/externalresources/storages/pgstroage"
	"os"
	"testing"
)

func ExamplePG(tb testing.TB) *pgstroage.PG {
	return pgstorage.NewPG(DatabaseURL(tb))
}

func TestPG(t *testing.T) {
	t.Run(``, func(t *testing.T) {





	})
}


const CreateTemporaryTableQuery = `
CREATE TEMPORARY TABLE ? (
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


		if _, err := db.Exec(`DROP SCHEMA public CASCADE`); err != nil {
			t.Fatal(err)
			return
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