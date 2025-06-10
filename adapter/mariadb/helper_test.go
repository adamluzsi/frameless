package mariadb_test

import (
	"context"
	"sync"
	"testing"

	"go.llib.dev/frameless/adapter/mariadb"
	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/testcase/assert"
)

var (
	_Connection     mariadb.Connection
	mutexConnection sync.Mutex
)

func GetConnection(tb testing.TB) mariadb.Connection {
	mutexConnection.Lock()
	defer mutexConnection.Unlock()
	if zerokit.IsZero(_Connection) {
		conn, err := mariadb.Connect(DatabaseDSN(tb))
		assert.NoError(tb, err)
		assert.NotNil(tb, conn)
		_Connection = conn
	}
	assert.NoError(tb, _Connection.DB.Ping())
	return _Connection
}

var rnd = random.New(random.CryptoSeed{})

func NewEntityRepository(tb testing.TB) *mariadb.Repository[Entity, EntityID] {
	cm := GetConnection(tb)
	MigrateEntity(tb, cm)
	return &mariadb.Repository[Entity, EntityID]{
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

	Prepare: func(ctx context.Context, f *testent.Foo) error {
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
	const envKey = "MARIADB_DATABASE_DSN"
	u, ok, err := env.Lookup[string](envKey)
	assert.NoError(tb, err)
	if !ok {
		tb.Skipf("env variable is missing %s", envKey)
	}
	return u
}

type EntityID int

type Entity struct {
	ID  EntityID `ext:"ID"`
	Foo string
	Bar string
	Baz string
}

type EntityDTO struct {
	ID  EntityID `ext:"ID" json:"id"`
	Foo string   `json:"foo"`
	Bar string   `json:"bar"`
	Baz string   `json:"baz"`
}

func EntityMapping() flsql.Mapping[Entity, EntityID] {
	var (
		idc int = 1
		m   sync.Mutex
	)
	var newID = func() EntityID {
		m.Lock()
		defer m.Unlock()
		idc++
		return EntityID(idc)
	}
	return flsql.Mapping[Entity, EntityID]{
		TableName: "test_entities",

		QueryID: func(id EntityID) (flsql.QueryArgs, error) {
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
				func(v *Entity, scan flsql.Scanner) error {
					return scan.Scan(&v.ID, &v.Foo, &v.Bar, &v.Baz)
				}
		},

		Prepare: func(ctx context.Context, e *Entity) error {
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
    id  INT      NOT NULL AUTO_INCREMENT PRIMARY KEY,
    foo LONGTEXT NOT NULL,
    bar LONGTEXT NOT NULL,
    baz LONGTEXT NOT NULL
);
`

const testMigrateDOWN = `
DROP TABLE IF EXISTS test_entities;
`
