package spechelper

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
	"github.com/stretchr/testify/require"
)

type TestEntity struct {
	ID  string `ext:"ID"`
	Foo string
	Bar string
	Baz string
}

func MakeTestEntity(tb testing.TB) TestEntity {
	te := tb.(*testcase.T).Random.Make(TestEntity{}).(TestEntity)
	te.ID = ""
	return te
}

func TestEntityMapping() postgresql.Mapper[TestEntity, string] {
	var counter int
	return postgresql.Mapper[TestEntity, string]{
		Table:   "test_entities",
		ID:      "id",
		Columns: []string{`id`, `foo`, `bar`, `baz`},
		NewIDFn: func(ctx context.Context) (string, error) {
			counter++
			rnd := random.New(random.CryptoSeed{})
			rndstr := rnd.StringNWithCharset(8, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
			return fmt.Sprintf("%d-%s", counter, rndstr), nil
		},
		ToArgsFn: func(ent *TestEntity) ([]interface{}, error) {
			return []interface{}{ent.ID, ent.Foo, ent.Bar, ent.Baz}, nil
		},
		MapFn: func(s iterators.SQLRowScanner) (TestEntity, error) {
			var ent TestEntity
			return ent, s.Scan(&ent.ID, &ent.Foo, &ent.Bar, &ent.Baz)
		},
	}
}

func MigrateTestEntity(tb testing.TB, cm postgresql.ConnectionManager) {
	ctx := context.Background()
	c, err := cm.Connection(ctx)
	require.Nil(tb, err)
	_, err = c.ExecContext(ctx, testMigrateDOWN)
	require.Nil(tb, err)
	_, err = c.ExecContext(ctx, testMigrateUP)
	require.Nil(tb, err)

	tb.Cleanup(func() {
		client, err := cm.Connection(ctx)
		require.Nil(tb, err)
		_, err = client.ExecContext(ctx, testMigrateDOWN)
		require.Nil(tb, err)
	})
}

const testMigrateUP = `
CREATE TABLE "test_entities" (
    id	TEXT	NOT	NULL	PRIMARY KEY,
	foo	TEXT	NOT	NULL,
	bar	TEXT	NOT	NULL,
	baz	TEXT	NOT	NULL
);
`

const testMigrateDOWN = `
DROP TABLE IF EXISTS "test_entities";
`
