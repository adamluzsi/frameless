package postgresql_test

import (
	"io"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/testcase/assert"
)

func NewTestEntityRepository(tb testing.TB) *postgresql.Repository[spechelper.TestEntity, string] {
	cm := NewConnectionManager(tb)
	spechelper.MigrateTestEntity(tb, cm)
	return &postgresql.Repository[spechelper.TestEntity, string]{
		Mapping:           spechelper.TestEntityMapping(),
		ConnectionManager: cm,
	}
}

func NewConnectionManager(tb testing.TB) postgresql.ConnectionManager {
	cm, err := postgresql.NewConnectionManagerWithDSN(spechelper.DatabaseDSN(tb))
	assert.NoError(tb, err)
	//connection, err := cm.Connection(context.Background())
	//assert.NoError(tb, err)
	//_, err = connection.ExecContext(context.Background(), "SELECT")
	//assert.NoError(tb, err)
	return cm
}

func deferClose(tb testing.TB, closer io.Closer) {
	tb.Cleanup(func() { _ = closer.Close() })
}
