package postgresql_test

import (
	"context"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/spechelper/testent"
	"github.com/adamluzsi/testcase/random"
	"sync"
	"testing"

	"go.llib.dev/frameless/adapters/postgresql/internal/spechelper"

	"go.llib.dev/frameless/adapters/postgresql"
	"github.com/adamluzsi/testcase/assert"
)

func NewTestEntityRepository(tb testing.TB) *postgresql.Repository[spechelper.TestEntity, string] {
	cm := GetConnection(tb)
	spechelper.MigrateTestEntity(tb, cm)
	return &postgresql.Repository[spechelper.TestEntity, string]{
		Mapping:    spechelper.TestEntityMapping(),
		Connection: cm,
	}
}

var (
	Connection postgresql.Connection
	mutexConnection sync.Mutex
)

func GetConnection(tb testing.TB) postgresql.Connection {
	mutexConnection.Lock()
	defer mutexConnection.Unlock()
	if Connection != nil {
		return Connection
	}
	cm, err := postgresql.Connect(spechelper.DatabaseDSN(tb))
	assert.NoError(tb, err)
	assert.NotNil(tb, cm)
	Connection = cm
	return cm
}

func MigrateFoo(tb testing.TB, c postgresql.Connection) {
	ctx := context.Background()
	_, err := c.ExecContext(ctx, FooMigrateDOWN)
	assert.Nil(tb, err)
	_, err = c.ExecContext(ctx, FooMigrateUP)
	assert.Nil(tb, err)
	tb.Cleanup(func() {
		_, err := c.ExecContext(ctx, FooMigrateDOWN)
		assert.Nil(tb, err)
	})
}

const FooMigrateUP = `
CREATE TABLE IF NOT EXISTS "foos" (
    id	TEXT	NOT	NULL	PRIMARY KEY,
	foo	TEXT	NOT	NULL,
	bar	TEXT	NOT	NULL,
	baz	TEXT	NOT	NULL
);
`

const FooMigrateDOWN = `
DROP TABLE IF EXISTS "foos";
`

var FooMapping = postgresql.Mapper[testent.Foo, testent.FooID]{
	Table:   "foos",
	ID:      "id",
	Columns: []string{"id", "foo", "bar", "baz"},
	ToArgsFn: func(ptr *testent.Foo) ([]interface{}, error) {
		return []any{ptr.ID, ptr.Foo, ptr.Bar, ptr.Baz}, nil
	},
	MapFn: func(scanner iterators.SQLRowScanner) (testent.Foo, error) {
		var foo testent.Foo
		err := scanner.Scan(&foo.ID, &foo.Foo, &foo.Bar, &foo.Baz)
		return foo, err
	},
	NewIDFn: func(ctx context.Context) (testent.FooID, error) {
		return testent.FooID(random.New(random.CryptoSeed{}).UUID()), nil
	},
}
