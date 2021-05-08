package postgresql_test

import (
	"os"
	"testing"
)

func GetDatabaseURL(tb testing.TB) string {
	databaseURL, ok := os.LookupEnv(`DATABASE_URL`)
	if !ok {
		tb.Skip(`DATABASE_URL missing`)
	}
	return databaseURL
}
