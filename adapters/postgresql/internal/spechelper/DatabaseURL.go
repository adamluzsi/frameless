package spechelper

import (
	"os"
	"testing"
)

func DatabaseDSN(tb testing.TB) string {
	const envKey = `PG_DATABASE_DSN`
	databaseURL, ok := os.LookupEnv(envKey)
	if !ok {
		tb.Skipf(`%s env variable is missing`, envKey)
	}
	return databaseURL
}

func DatabaseURL(tb testing.TB) string {
	const envKey = `PG_DATABASE_URL`
	databaseURL, ok := os.LookupEnv(envKey)
	if !ok {
		tb.Skipf(`%s env variable is missing`, envKey)
	}
	return databaseURL
}
