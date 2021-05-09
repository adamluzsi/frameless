package postgresql_test

import (
	"os"
	"testing"
)

func GetDatabaseURL(tb testing.TB) string {
	const envKey = `POSTGRES_DATABASE_URL`
	databaseURL, ok := os.LookupEnv(envKey)
	if !ok {
		tb.Skipf(`%s env variable is missing`, envKey)
	}
	return databaseURL
}
