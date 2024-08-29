package mysql_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapter/mysql"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/testcase/assert"
)

var (
	Connection      flsql.Connection
	mutexConnection sync.Mutex
)

func GetConnection(tb testing.TB) flsql.Connection {
	mutexConnection.Lock()
	defer mutexConnection.Unlock()
	if Connection != nil {
		return Connection
	}
	cm, err := mysql.Connect(DatabaseDSN(tb))
	assert.NoError(tb, err)
	assert.NotNil(tb, cm)
	Connection = cm
	return cm
}

var rnd = random.New(random.CryptoSeed{})

func NewEntityRepository(tb testing.TB) *mysql.Repository[Entity, EntityID] {
	cm := GetConnection(tb)
	MigrateEntity(tb, cm)
	return &mysql.Repository[Entity, EntityID]{
		Mapping:    EntityMapping(),
		Connection: cm,
	}
}

func MigrateFoo(tb testing.TB, c flsql.Connection) {
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
			func(f *testent.Foo, sf flsql.ScanFunc) error {
				return sf(&f.ID, &f.Foo, &f.Bar, &f.Baz)
			}
	},

	ToID: func(id testent.FooID) (map[flsql.ColumnName]any, error) {
		return map[flsql.ColumnName]any{"id": id}, nil
	},

	ToArgs: func(f testent.Foo) (map[flsql.ColumnName]any, error) {
		return map[flsql.ColumnName]any{
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

	GetID: func(f testent.Foo) testent.FooID { return f.ID },
}

func MakeContext(testing.TB) context.Context { return context.Background() }

func MakeString(tb testing.TB) string {
	return tb.(*testcase.T).Random.String()
}

func DatabaseDSN(tb testing.TB) string {
	const envKey = "MARIADB_DATABASE_DSN"
	u, ok, err := env.Lookup[string](envKey)
	assert.NoError(tb, err)
	if !ok {
		tb.Skipf("env variable is missing %s", envKey)
	}
	return u
}

type EntityID string

type Entity struct {
	ID  EntityID `ext:"ID"`
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
	ID  EntityID `ext:"ID" json:"id"`
	Foo string   `json:"foo"`
	Bar string   `json:"bar"`
	Baz string   `json:"baz"`
}

type EntityJSONMapping struct{}

func (n EntityJSONMapping) ToDTO(ent Entity) (EntityDTO, error) {
	return EntityDTO{ID: ent.ID, Foo: ent.Foo, Bar: ent.Bar, Baz: ent.Baz}, nil
}

func (n EntityJSONMapping) ToEnt(dto EntityDTO) (Entity, error) {
	return Entity{ID: dto.ID, Foo: dto.Foo, Bar: dto.Bar, Baz: dto.Baz}, nil
}

func EntityMapping() flsql.Mapping[Entity, EntityID] {
	var (
		idc int = 1
		m   sync.Mutex
		rnd = random.New(random.CryptoSeed{})
	)
	var newID = func() EntityID {
		m.Lock()
		defer m.Unlock()
		idc++
		rndstr := rnd.StringNWithCharset(8, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
		return EntityID(fmt.Sprintf("%d-%s", idc, rndstr))
	}
	return flsql.Mapping[Entity, EntityID]{
		TableName: "test_entities",

		ToID: func(id EntityID) (map[flsql.ColumnName]any, error) {
			return map[flsql.ColumnName]any{"id": id}, nil
		},

		ToArgs: func(e Entity) (map[flsql.ColumnName]any, error) {
			return map[flsql.ColumnName]any{
				`id`:  e.ID,
				`foo`: e.Foo,
				`bar`: e.Bar,
				`baz`: e.Baz,
			}, nil
		},

		ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[Entity]) {
			return []flsql.ColumnName{`id`, `foo`, `bar`, `baz`},
				func(v *Entity, scan flsql.ScanFunc) error {
					return scan(&v.ID, &v.Foo, &v.Bar, &v.Baz)
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

func MigrateEntity(tb testing.TB, cm flsql.Connection) {
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
CREATE TABLE test_entities (
    id  VARCHAR(255) NOT NULL PRIMARY KEY,
    foo LONGTEXT     NOT NULL,
    bar LONGTEXT     NOT NULL,
    baz LONGTEXT     NOT NULL
);
`

const testMigrateDOWN = `
DROP TABLE IF EXISTS test_entities;
`
