package spechelper

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/postgresql"
	"github.com/stretchr/testify/require"
)

type TestEntity struct {
	ID  string `ext:"ID"`
	Foo string
	Bar string
	Baz string
}

func TestEntityMapping() postgresql.Mapper {
	return postgresql.Mapper{
		Table:   "test_entities",
		ID:      "id",
		Columns: []string{`id`, `foo`, `bar`, `baz`},
		NewIDFn: func(ctx context.Context) (interface{}, error) {
			return fixtures.Random.StringN(42), nil
		},
		ToArgsFn: func(ptr interface{}) ([]interface{}, error) {
			ent := ptr.(*TestEntity)
			return []interface{}{ent.ID, ent.Foo, ent.Bar, ent.Baz}, nil
		},
		MapFn: func(s iterators.SQLRowScanner, ptr interface{}) error {
			ent := ptr.(*TestEntity)
			return s.Scan(&ent.ID, &ent.Foo, &ent.Bar, &ent.Baz)
		},
	}
}

func MigrateTestEntity(tb testing.TB, cm postgresql.ConnectionManager) {
	ctx := context.Background()
	c, err := cm.Connection(ctx)
	require.Nil(tb, err)
	_, err = c.ExecContext(ctx, storageTestMigrateDOWN)
	require.Nil(tb, err)
	_, err = c.ExecContext(ctx, storageTestMigrateUP)
	require.Nil(tb, err)

	tb.Cleanup(func() {
		client, err := cm.Connection(ctx)
		require.Nil(tb, err)
		_, err = client.ExecContext(ctx, storageTestMigrateDOWN)
		require.Nil(tb, err)
	})
}

const storageTestMigrateUP = `
CREATE TABLE "test_entities" (
    id	TEXT	NOT	NULL	PRIMARY KEY,
	foo	TEXT	NOT	NULL,
	bar	TEXT	NOT	NULL,
	baz	TEXT	NOT	NULL
);
`

const storageTestMigrateDOWN = `
DROP TABLE IF EXISTS "test_entities";
`
