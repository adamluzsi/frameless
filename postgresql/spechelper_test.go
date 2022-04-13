package postgresql_test

import (
	"io"
	"testing"

	"github.com/adamluzsi/frameless/postgresql"
	psh "github.com/adamluzsi/frameless/postgresql/spechelper"
	"github.com/stretchr/testify/assert"
)

func NewStorage[Ent, ID any](tb testing.TB) *postgresql.Storage[Ent, ID] {
	stg := postgresql.NewStorageByDSN[Ent, ID](psh.TestEntity{}, psh.TestEntityMapping(), psh.DatabaseURL(tb))
	psh.MigrateTestEntity(tb, stg.ConnectionManager)
	deferClose(tb, stg)
	return stg
}

func deferClose(tb testing.TB, closer io.Closer) {
	tb.Cleanup(func() { assert.NoError(tb, closer.Close()) })
}
