package postgresql_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapters/postgresql"
	"go.llib.dev/testcase/assert"
)

func NewEntityRepository(tb testing.TB) *postgresql.Repository[Entity, string] {
	cm := GetConnection(tb)
	MigrateEntity(tb, cm)
	return &postgresql.Repository[Entity, string]{
		Mapping:    EntityMapping(),
		Connection: cm,
	}
}

var (
	Connection      postgresql.Connection
	mutexConnection sync.Mutex
)

func GetConnection(tb testing.TB) postgresql.Connection {
	mutexConnection.Lock()
	defer mutexConnection.Unlock()
	if Connection != nil {
		return Connection
	}
	cm, err := postgresql.Connect(DatabaseDSN(tb))
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

var FooMapping = postgresql.Mapping[testent.Foo, testent.FooID]{
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

func MakeContext(testing.TB) context.Context { return context.Background() }

func MakeString(tb testing.TB) string {
	return tb.(*testcase.T).Random.String()
}

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

type Entity struct {
	ID  string `ext:"ID"`
	Foo string
	Bar string
	Baz string
}

func MakeEntityFunc(tb testing.TB) func() Entity {
	return func() Entity {
		te := tb.(*testcase.T).Random.Make(Entity{}).(Entity)
		te.ID = ""
		return te
	}
}

type EntityDTO struct {
	ID  string `ext:"ID" json:"id"`
	Foo string `json:"foo"`
	Bar string `json:"bar"`
	Baz string `json:"baz"`
}

type EntityJSONMapping struct{}

func (n EntityJSONMapping) ToDTO(ent Entity) (EntityDTO, error) {
	return EntityDTO{ID: ent.ID, Foo: ent.Foo, Bar: ent.Bar, Baz: ent.Baz}, nil
}

func (n EntityJSONMapping) ToEnt(dto EntityDTO) (Entity, error) {
	return Entity{ID: dto.ID, Foo: dto.Foo, Bar: dto.Bar, Baz: dto.Baz}, nil
}

func EntityMapping() postgresql.Mapping[Entity, string] {
	var counter int
	return postgresql.Mapping[Entity, string]{
		Table: "test_entities",
		ID:    "id",
		NewIDFn: func(ctx context.Context) (string, error) {
			counter++
			rnd := random.New(random.CryptoSeed{})
			rndstr := rnd.StringNWithCharset(8, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
			return fmt.Sprintf("%d-%s", counter, rndstr), nil
		},
		Columns: []string{`id`, `foo`, `bar`, `baz`},
		ToArgsFn: func(ent *Entity) ([]interface{}, error) {
			return []interface{}{ent.ID, ent.Foo, ent.Bar, ent.Baz}, nil
		},
		MapFn: func(s iterators.SQLRowScanner) (Entity, error) {
			var ent Entity
			return ent, s.Scan(&ent.ID, &ent.Foo, &ent.Bar, &ent.Baz)
		},
	}
}

func MigrateEntity(tb testing.TB, cm postgresql.Connection) {
	ctx := context.Background()
	_, err := cm.ExecContext(ctx, testMigrateDOWN)
	assert.Nil(tb, err)
	_, err = cm.ExecContext(ctx, testMigrateUP)
	assert.Nil(tb, err)

	tb.Cleanup(func() {
		_, err := cm.ExecContext(ctx, testMigrateDOWN)
		assert.Nil(tb, err)
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
