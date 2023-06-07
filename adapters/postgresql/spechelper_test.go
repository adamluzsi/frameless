package postgresql_test

import (
	"context"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper/testent"
	"github.com/adamluzsi/testcase/random"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/testcase/assert"
)

func NewTestEntityRepository(tb testing.TB) *postgresql.Repository[spechelper.TestEntity, string] {
	cm := GetConnectionManager(tb)
	spechelper.MigrateTestEntity(tb, cm)
	return &postgresql.Repository[spechelper.TestEntity, string]{
		Mapping: spechelper.TestEntityMapping(),
		CM:      cm,
	}
}

var CM postgresql.ConnectionManager

func GetConnectionManager(tb testing.TB) postgresql.ConnectionManager {
	if CM != nil {
		return CM
	}
	cm, err := postgresql.NewConnectionManager(spechelper.DatabaseDSN(tb))
	assert.NoError(tb, err)
	assert.NotNil(tb, cm)
	CM = cm
	return cm
}

func MigrateFoo(tb testing.TB, cm postgresql.ConnectionManager) {
	ctx := context.Background()
	c, err := cm.Connection(ctx)
	assert.Nil(tb, err)
	_, err = c.ExecContext(ctx, FooMigrateDOWN)
	assert.Nil(tb, err)
	_, err = c.ExecContext(ctx, FooMigrateUP)
	assert.Nil(tb, err)

	tb.Cleanup(func() {
		client, err := cm.Connection(ctx)
		assert.Nil(tb, err)
		_, err = client.ExecContext(ctx, FooMigrateDOWN)
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
