package postgresql_test

import (
	"io"
	"testing"

	"github.com/adamluzsi/frameless/postgresql"
	psh "github.com/adamluzsi/frameless/postgresql/spechelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func NewStorage(tb testing.TB) *postgresql.Storage {
	stg, err := postgresql.NewStorageByDSN(psh.TestEntity{}, psh.TestEntityMapping(), psh.DatabaseURL(tb))
	require.NoError(tb, err)
	psh.MigrateTestEntity(tb, stg.ConnectionManager)
	deferClose(tb, stg)
	tb.Cleanup(func() { stg.Close() })
	return stg
}

func deferClose(tb testing.TB, closer io.Closer) {
	tb.Cleanup(func() { assert.NoError(tb, closer.Close()) })
}
