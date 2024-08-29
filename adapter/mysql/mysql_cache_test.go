package mysql_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/adapter/mysql"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/spechelper/testent"
)

func TestRepository_cacheEntityRepository(t *testing.T) {
	cm := GetConnection(t)
	MigrateFooCache(t, cm)
	chcRepo := FooCacheRepository{Connection: cm}
	cachecontracts.EntityRepository[testent.Foo, testent.FooID](chcRepo.Entities(), cm)
}

func TestRepository_cacheHitRepository(t *testing.T) {
	cm := GetConnection(t)
	MigrateFooCache(t, cm)
	chcRepo := FooCacheRepository{Connection: cm}
	cachecontracts.HitRepository[testent.FooID](chcRepo.Hits(), cm)
}

func TestRepository_cacheCache(t *testing.T) {
	logger.Testing(t)

	cm := GetConnection(t)
	MigrateFooCache(t, cm)

	src := memory.NewRepository[testent.Foo, testent.FooID](memory.NewMemory())
	chcRepo := FooCacheRepository{Connection: cm}

	chc := &cache.Cache[testent.Foo, testent.FooID]{
		Source:                  src,
		Repository:              chcRepo,
		CachedQueryInvalidators: nil,
	}

	cachecontracts.Cache[testent.Foo, testent.FooID](chc, src, chcRepo).Test(t)
}

func splitQuery(query string) []string {
	var queries []string
	for _, q := range strings.Split(query, ";") {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		queries = append(queries, q)
	}
	return queries
}

func MigrateFooCache(tb testing.TB, c flsql.Connection) {
	ctx := context.Background()

	for _, query := range splitQuery(FooCacheMigrateDOWN) {
		_, err := c.ExecContext(ctx, query)
		assert.Nil(tb, err)
	}

	for _, query := range splitQuery(FooCacheMigrateUP) {
		_, err := c.ExecContext(ctx, query)
		assert.Nil(tb, err)
	}

	tb.Cleanup(func() {
		for _, query := range splitQuery(FooCacheMigrateDOWN) {
			_, err := c.ExecContext(ctx, query)
			assert.Nil(tb, err)
		}
	})
}

const FooCacheMigrateUP = `
CREATE TABLE IF NOT EXISTS cache_foos (
    id  VARCHAR(255) NOT NULL PRIMARY KEY,
	foo LONGTEXT     NOT NULL,
	bar LONGTEXT     NOT NULL,
	baz LONGTEXT     NOT NULL
);

CREATE TABLE IF NOT EXISTS cache_foo_hits (
	id  VARCHAR(255) NOT NULL PRIMARY KEY,
	ids JSON NOT     NULL,
	ts  TIMESTAMP    NOT NULL
);
`

const FooCacheMigrateDOWN = `
DROP TABLE IF EXISTS cache_foos;
DROP TABLE IF EXISTS cache_foo_hits;
`

type FooCacheRepository struct {
	flsql.Connection
	EntityRepository cache.EntityRepository[testent.Foo, testent.FooID]
	HitRepository    cache.HitRepository[testent.FooID]
}

func (cr FooCacheRepository) Entities() cache.EntityRepository[testent.Foo, testent.FooID] {
	return mysql.Repository[testent.Foo, testent.FooID]{
		Mapping: flsql.Mapping[testent.Foo, testent.FooID]{
			TableName: "cache_foos",

			ToID: func(id testent.FooID) (map[flsql.ColumnName]any, error) {
				return map[flsql.ColumnName]any{"id": id}, nil
			},

			ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[testent.Foo]) {
				return []flsql.ColumnName{"id", "foo", "bar", "baz"}, func(foo *testent.Foo, scan flsql.ScanFunc) error {
					return scan(&foo.ID, &foo.Foo, &foo.Bar, &foo.Baz)
				}
			},

			ToArgs: func(foo testent.Foo) (map[flsql.ColumnName]any, error) {
				return map[flsql.ColumnName]any{
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
	return mysql.Repository[cache.Hit[testent.FooID], cache.HitID]{
		Mapping: flsql.Mapping[cache.Hit[testent.FooID], cache.HitID]{
			TableName: "cache_foo_hits",

			ToID: func(id string) (map[flsql.ColumnName]any, error) {
				return map[flsql.ColumnName]any{"id": id}, nil
			},

			ToArgs: func(h cache.Hit[testent.FooID]) (map[flsql.ColumnName]any, error) {
				return map[flsql.ColumnName]any{
					"id":  h.QueryID,
					"ids": mysql.JSON(&h.EntityIDs),
					"ts":  sql.NullTime{Time: h.Timestamp, Valid: true},
				}, nil
			},

			ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[cache.Hit[testent.FooID]]) {
				return []flsql.ColumnName{"id", "ids", "ts"}, func(v *cache.Hit[testent.FooID], scan flsql.ScanFunc) error {
					err := scan(&v.QueryID, mysql.JSON(&v.EntityIDs), mysql.Timestamp(&v.Timestamp))
					if err != nil {
						return err
					}
					if !v.Timestamp.IsZero() {
						v.Timestamp = v.Timestamp.UTC()
					}
					return err
				}
			},

			GetID: func(h cache.Hit[testent.FooID]) string {
				return h.QueryID
			},
		},
		Connection: cr.Connection,
	}
}
