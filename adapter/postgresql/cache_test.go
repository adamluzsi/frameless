package postgresql_test

import (
	"context"
	"testing"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/spechelper/testent"
)

func TestRepository_cache(t *testing.T) {
	logger.Testing(t)

	cm := GetConnection(t)
	src := memory.NewRepository[testent.Foo, testent.FooID](memory.NewMemory())
	chcRepo := FooCacheRepository{Connection: cm}

	MigrateFooCache(t, cm)

	chc := &cache.Cache[testent.Foo, testent.FooID]{
		Source:                  src,
		Repository:              chcRepo,
		CachedQueryInvalidators: nil,
	}

	cachecontracts.Cache[testent.Foo, testent.FooID](chc, src, chcRepo).Test(t)
}

func MigrateFooCache(tb testing.TB, c postgresql.Connection) {
	ctx := context.Background()
	_, err := c.ExecContext(ctx, FooCacheMigrateDOWN)
	assert.Nil(tb, err)
	_, err = c.ExecContext(ctx, FooCacheMigrateUP)
	assert.Nil(tb, err)

	tb.Cleanup(func() {
		_, err := c.ExecContext(ctx, FooCacheMigrateDOWN)
		assert.Nil(tb, err)
	})
}

const FooCacheMigrateUP = `
CREATE TABLE IF NOT EXISTS "cache_foos" (
    id	TEXT	NOT	NULL	PRIMARY KEY,
	foo	TEXT	NOT	NULL,
	bar	TEXT	NOT	NULL,
	baz	TEXT	NOT	NULL
);

CREATE TABLE IF NOT EXISTS "cache_foo_hits" (
	id	TEXT	NOT	NULL	PRIMARY KEY,
	ids	TEXT[],
	ts	TIMESTAMP WITH TIME ZONE NOT NULL
)
`

const FooCacheMigrateDOWN = `
DROP TABLE IF EXISTS "cache_foos";
DROP TABLE IF EXISTS "cache_foo_hits";
`

type FooCacheRepository struct {
	postgresql.Connection
	EntityRepository cache.EntityRepository[testent.Foo, testent.FooID]
	HitRepository    cache.HitRepository[testent.FooID]
}

func (cr FooCacheRepository) Entities() cache.EntityRepository[testent.Foo, testent.FooID] {
	return postgresql.Repository[testent.Foo, testent.FooID]{
		Mapping: flsql.Mapping[testent.Foo, testent.FooID]{
			TableName: "cache_foos",

			QueryID: func(id testent.FooID) (flsql.QueryArgs, error) {
				return flsql.QueryArgs{"id": id}, nil
			},

			ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[testent.Foo]) {
				return []flsql.ColumnName{"id", "foo", "bar", "baz"}, func(foo *testent.Foo, s flsql.Scanner) error {
					return s.Scan(&foo.ID, &foo.Foo, &foo.Bar, &foo.Baz)
				}
			},

			ToArgs: func(foo testent.Foo) (flsql.QueryArgs, error) {
				return flsql.QueryArgs{
					"id":  foo.ID,
					"foo": foo.Foo,
					"bar": foo.Bar,
					"baz": foo.Baz,
				}, nil
			},

			CreatePrepare: func(ctx context.Context, f *testent.Foo) error {
				if zerokit.IsZero(f.ID) {
					f.ID = testent.FooID(random.New(random.CryptoSeed{}).UUID())
				}
				return nil
			},
		},
		Connection: cr.Connection,
	}
}

func (cr FooCacheRepository) Hits() cache.HitRepository[testent.FooID] {
	return postgresql.Repository[cache.Hit[testent.FooID], cache.HitID]{
		Mapping: flsql.Mapping[cache.Hit[testent.FooID], cache.HitID]{
			TableName: "cache_foo_hits",

			QueryID: func(id cache.HitID) (flsql.QueryArgs, error) {
				return flsql.QueryArgs{"id": id}, nil
			},

			ToArgs: func(h cache.Hit[testent.FooID]) (flsql.QueryArgs, error) {
				return flsql.QueryArgs{
					"id":  h.ID,
					"ids": &h.EntityIDs,
					"ts":  h.Timestamp,
				}, nil
			},

			ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[cache.Hit[testent.FooID]]) {
				return []flsql.ColumnName{"id", "ids", "ts"}, func(v *cache.Hit[testent.FooID], s flsql.Scanner) error {
					var ids []string
					err := s.Scan(&v.ID, &ids, &v.Timestamp)
					for _, id := range ids {
						v.EntityIDs = append(v.EntityIDs, testent.FooID(id))
					}
					v.Timestamp = v.Timestamp.UTC()
					return err
				}
			},

			ID: func(h *cache.Hit[testent.FooID]) *cache.HitID {
				return &h.ID
			},
		},
		Connection: cr.Connection,
	}
}
