package postgresql_test

import (
	"io"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	psh "github.com/adamluzsi/frameless/adapters/postgresql/spechelper"
	"github.com/stretchr/testify/assert"
)

func NewTestEntityRepository(tb testing.TB) *postgresql.Repository[psh.TestEntity, string] {
	stg := postgresql.NewRepositoryByDSN[psh.TestEntity, string](psh.TestEntityMapping(), psh.DatabaseURL(tb))
	psh.MigrateTestEntity(tb, stg.ConnectionManager)
	deferClose(tb, stg)
	return stg
}

func deferClose(tb testing.TB, closer io.Closer) {
	tb.Cleanup(func() { assert.NoError(tb, closer.Close()) })
}
