package postgresql_test

import (
	"context"
	"testing"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/adapters/postgresql"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/spechelper/testent"
)

func TestRepository_cache(t *testing.T) {
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
		Mapping: postgresql.Mapping[testent.Foo, testent.FooID]{
			Table:   "cache_foos",
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
		},
		Connection: cr.Connection,
	}
}

func (cr FooCacheRepository) Hits() cache.HitRepository[testent.FooID] {
	return postgresql.Repository[cache.Hit[testent.FooID], cache.HitID]{
		Mapping: postgresql.Mapping[cache.Hit[testent.FooID], cache.HitID]{
			Table:   "cache_foo_hits",
			ID:      "id",
			Columns: []string{"id", "ids", "ts"},
			ToArgsFn: func(ptr *cache.Hit[testent.FooID]) ([]interface{}, error) {
				ptr.Timestamp = ptr.Timestamp.UTC()
				return []any{ptr.QueryID, &ptr.EntityIDs, ptr.Timestamp}, nil
			},
			MapFn: func(scanner iterators.SQLRowScanner) (cache.Hit[testent.FooID], error) {
				var (
					ent cache.Hit[testent.FooID]
					ids []string
				)
				err := scanner.Scan(&ent.QueryID, &ids, &ent.Timestamp)
				for _, id := range ids {
					ent.EntityIDs = append(ent.EntityIDs, testent.FooID(id))
				}
				ent.Timestamp = ent.Timestamp.UTC()
				return ent, err
			},
			NewIDFn: func(ctx context.Context) (_ cache.HitID, _ error) { return },
		},
		Connection: cr.Connection,
	}
}
