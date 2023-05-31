package postgresql_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"
	"github.com/adamluzsi/frameless/pkg/cache"
	"github.com/adamluzsi/frameless/pkg/cache/cachecontracts"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper/testent"

	"github.com/lib/pq"
)

func TestRepository_cache(t *testing.T) {
	db, err := sql.Open("postgres", spechelper.DatabaseURL(t))
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	cm, err := postgresql.NewConnectionManagerWithDSN(spechelper.DatabaseURL(t))
	assert.NoError(t, err)

	src := memory.NewRepository[testent.Foo, testent.FooID](memory.NewMemory())

	MigrateFooCache(t, cm)

	chcRepo := FooCacheRepository{ConnectionManager: cm}

	chc := &cache.Cache[testent.Foo, testent.FooID]{
		Source:                  src,
		Repository:              chcRepo,
		CachedQueryInvalidators: nil,
	}

	rnd := random.New(random.CryptoSeed{})

	cachecontracts.Cache[testent.Foo, testent.FooID](func(tb testing.TB) cachecontracts.CacheSubject[testent.Foo, testent.FooID] {
		return cachecontracts.CacheSubject[testent.Foo, testent.FooID]{
			Cache:       chc,
			Source:      src,
			Repository:  chcRepo,
			MakeContext: context.Background,
			MakeEntity: func() testent.Foo {
				v := rnd.Make(testent.Foo{}).(testent.Foo)
				v.ID = ""
				return v
			},
			ChangeEntity: nil,
		}
	}).Test(t)
}

func MigrateFooCache(tb testing.TB, cm postgresql.ConnectionManager) {
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

const FooMigrateDOWN = `
DROP TABLE IF EXISTS "cache_foos";
DROP TABLE IF EXISTS "cache_foo_hits";
`

type FooCacheRepository struct {
	postgresql.ConnectionManager
	EntityRepository cache.EntityRepository[testent.Foo, testent.FooID]
	HitRepository    cache.HitRepository[testent.FooID]
}

func (cr FooCacheRepository) Entities() cache.EntityRepository[testent.Foo, testent.FooID] {
	return postgresql.Repository[testent.Foo, testent.FooID]{
		Mapping: postgresql.Mapper[testent.Foo, testent.FooID]{
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
		ConnectionManager: cr.ConnectionManager,
	}
}

func (cr FooCacheRepository) Hits() cache.HitRepository[testent.FooID] {
	return postgresql.Repository[cache.Hit[testent.FooID], cache.HitID]{
		Mapping: postgresql.Mapper[cache.Hit[testent.FooID], cache.HitID]{
			Table:   "cache_foo_hits",
			ID:      "id",
			Columns: []string{"id", "ids", "ts"},
			ToArgsFn: func(ptr *cache.Hit[testent.FooID]) ([]interface{}, error) {
				ptr.Timestamp = ptr.Timestamp.UTC()
				return []any{ptr.QueryID, pq.Array(ptr.EntityIDs), ptr.Timestamp}, nil
			},
			MapFn: func(scanner iterators.SQLRowScanner) (cache.Hit[testent.FooID], error) {
				var (
					ent cache.Hit[testent.FooID]
					ids []string
				)
				err := scanner.Scan(&ent.QueryID, pq.Array(&ids), &ent.Timestamp)
				for _, id := range ids {
					ent.EntityIDs = append(ent.EntityIDs, testent.FooID(id))
				}
				ent.Timestamp = ent.Timestamp.UTC()
				return ent, err
			},
			NewIDFn: func(ctx context.Context) (_ cache.HitID, _ error) { return },
		},
		ConnectionManager: cr.ConnectionManager,
	}
}
