package postgresql_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/testcase/assert"
)

var rnd = random.New(random.CryptoSeed{})

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
	if !zerokit.IsZero(Connection) {
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

var FooMapping = flsql.Mapping[testent.Foo, testent.FooID]{
	TableName: "foos",

	ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[testent.Foo]) {
		return []flsql.ColumnName{"id", "foo", "bar", "baz"},
			func(f *testent.Foo, sf flsql.Scanner) error {
				return sf.Scan(&f.ID, &f.Foo, &f.Bar, &f.Baz)
			}
	},

	QueryID: func(id testent.FooID) (flsql.QueryArgs, error) {
		return flsql.QueryArgs{"id": id}, nil
	},

	ToArgs: func(f testent.Foo) (flsql.QueryArgs, error) {
		return flsql.QueryArgs{
			"id":  f.ID,
			"foo": f.Foo,
			"bar": f.Bar,
			"baz": f.Baz,
		}, nil
	},

	CreatePrepare: func(ctx context.Context, f *testent.Foo) error {
		if zerokit.IsZero(f.ID) {
			f.ID = testent.FooID(random.New(random.CryptoSeed{}).UUID())
		}
		return nil
	},

	ID: func(f *testent.Foo) *testent.FooID { return &f.ID },
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

func (n EntityJSONMapping) MapToDTO(_ context.Context, ent Entity) (EntityDTO, error) {
	return EntityDTO{ID: ent.ID, Foo: ent.Foo, Bar: ent.Bar, Baz: ent.Baz}, nil
}

func (n EntityJSONMapping) MapToENT(_ context.Context, dto EntityDTO) (Entity, error) {
	return Entity{ID: dto.ID, Foo: dto.Foo, Bar: dto.Bar, Baz: dto.Baz}, nil
}

func EntityMapping() flsql.Mapping[Entity, string] {
	var (
		idc int = 1
		m   sync.Mutex
		rnd = random.New(random.CryptoSeed{})
	)
	var newID = func() string {
		m.Lock()
		defer m.Unlock()
		idc++
		rndstr := rnd.StringNWithCharset(8, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
		return fmt.Sprintf("%d-%s", idc, rndstr)
	}
	return flsql.Mapping[Entity, string]{
		TableName: "test_entities",

		QueryID: func(id string) (flsql.QueryArgs, error) {
			return flsql.QueryArgs{"id": id}, nil
		},

		ToArgs: func(e Entity) (flsql.QueryArgs, error) {
			return flsql.QueryArgs{
				`id`:  e.ID,
				`foo`: e.Foo,
				`bar`: e.Bar,
				`baz`: e.Baz,
			}, nil
		},

		ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[Entity]) {
			return []flsql.ColumnName{`id`, `foo`, `bar`, `baz`},
				func(v *Entity, s flsql.Scanner) error {
					return s.Scan(&v.ID, &v.Foo, &v.Bar, &v.Baz)
				}
		},

		CreatePrepare: func(ctx context.Context, e *Entity) error {
			if zerokit.IsZero(e.ID) {
				e.ID = newID()
			}
			return nil
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
