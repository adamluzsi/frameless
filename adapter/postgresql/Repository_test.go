package postgresql_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/flsql"

	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontract"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/crud/crudtest"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func ExampleRepository() {
	type Entity struct {
		ID    int `ext:"ID"`
		Value string
	}

	mapping := flsql.Mapping[Entity, int]{
		TableName: "entities",

		QueryID: func(id int) (flsql.QueryArgs, error) {
			return flsql.QueryArgs{"id": id}, nil
		},

		ToArgs: func(e Entity) (flsql.QueryArgs, error) {
			return flsql.QueryArgs{
				`id`:    e.ID,
				`value`: e.Value,
			}, nil
		},

		ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[Entity]) {
			return []flsql.ColumnName{"id", "value"},
				func(v *Entity, s flsql.Scanner) error {
					return s.Scan(&v.ID, &v.Value)
				}
		},

		ID: func(e *Entity) *int {
			return &e.ID
		},
	}

	cm, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer cm.Close()

	repo := postgresql.Repository[Entity, int]{
		Connection: cm,
		Mapping:    mapping,
	}

	_ = repo
}

func TestRepository(t *testing.T) {
	mapping := EntityMapping()

	cm, err := postgresql.Connect(DatabaseURL(t))
	assert.NoError(t, err)

	subject := &postgresql.Repository[Entity, string]{
		Connection: cm,
		Mapping:    mapping,
	}

	MigrateEntity(t, cm)

	config := crudcontract.Config[Entity, string]{
		MakeContext: func(t testing.TB) context.Context {
			return context.Background()
		},
		SupportIDReuse:  true,
		SupportRecreate: true,

		ChangeEntity: nil, // test entity can be freely changed
	}

	testcase.RunSuite(t,
		crudcontract.Creator[Entity, string](subject, config),
		crudcontract.Finder[Entity, string](subject, config),
		crudcontract.Updater[Entity, string](subject, config),
		crudcontract.Deleter[Entity, string](subject, config),
		crudcontract.OnePhaseCommitProtocol[Entity, string](subject, subject.Connection),
	)
}

func TestRepository_mappingHasSchemaInTableName(t *testing.T) {
	cm := GetConnection(t)
	MigrateEntity(t, cm)

	mapper := EntityMapping()
	mapper.TableName = `public.` + mapper.TableName

	subject := postgresql.Repository[Entity, string]{
		Mapping:    mapper,
		Connection: cm,
	}

	crudcontract.Creator[Entity, string](subject, crudcontract.Config[Entity, string]{
		SupportIDReuse:  true,
		SupportRecreate: true,
	}).Test(t)
}

func TestRepository_implementsCacheEntityRepository(t *testing.T) {
	cm := GetConnection(t)
	MigrateEntity(t, cm)

	repo := postgresql.Repository[Entity, string]{
		Mapping:    EntityMapping(),
		Connection: cm,
	}

	cachecontract.EntityRepository[Entity, string](repo, cm).Test(t)
}

func TestRepository_canImplementCacheHitRepository(t *testing.T) {
	c := GetConnection(t)

	func(tb testing.TB, cm postgresql.Connection) {
		const testCacheHitMigrateUP = `CREATE TABLE "test_cache_hits" ( id TEXT PRIMARY KEY, ids TEXT[], ts TIMESTAMP WITH TIME ZONE );`
		const testCacheHitMigrateDOWN = `DROP TABLE IF EXISTS "test_cache_hits";`

		ctx := context.Background()
		_, err := c.ExecContext(ctx, testCacheHitMigrateDOWN)
		assert.NoError(tb, err)
		_, err = c.ExecContext(ctx, testCacheHitMigrateUP)
		assert.NoError(tb, err)

		tb.Cleanup(func() {
			_, err := c.ExecContext(ctx, testCacheHitMigrateDOWN)
			assert.NoError(tb, err)
		})
	}(t, c)

	hitRepo := postgresql.Repository[cache.Hit[string], cache.HitID]{
		Mapping: flsql.Mapping[cache.Hit[string], cache.HitID]{
			TableName: "test_cache_hits",

			QueryID: func(id cache.HitID) (flsql.QueryArgs, error) {
				return flsql.QueryArgs{"id": id}, nil
			},

			ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[cache.Hit[string]]) {
				return []flsql.ColumnName{"id", "ids", "ts"}, func(v *cache.Hit[string], s flsql.Scanner) error {
					if err := s.Scan(&v.ID, &v.EntityIDs, &v.Timestamp); err != nil {
						return err
					}
					v.Timestamp = v.Timestamp.UTC()
					return nil
				}
			},

			ToArgs: func(h cache.Hit[string]) (flsql.QueryArgs, error) {
				return flsql.QueryArgs{
					"id":  h.ID,
					"ids": h.EntityIDs,
					"ts":  h.Timestamp,
				}, nil
			},
		},
		Connection: c,
	}

	cachecontract.HitRepository[string](hitRepo, c, crudcontract.Config[cache.Hit[string], cache.HitID]{
		MakeEntity: func(tb testing.TB) cache.Hit[string] {
			t := tb.(*testcase.T)
			return cache.Hit[string]{
				ID: cache.HitID(t.Random.UUID()),
				EntityIDs: random.Slice(t.Random.IntBetween(0, 7), func() string {
					return t.Random.UUID()
				}),
				Timestamp: t.Random.Time().UTC(),
			}
		},
	}).Test(t)
}

func TestRepository_comprotoOnePhaseCommitProtocol(t *testing.T) {
	repo := &postgresql.Repository[testent.Foo, testent.FooID]{
		Connection: GetConnection(t),
		Mapping:    FooMapping,
	}
	MigrateFoo(t, repo.Connection)

	ctx := context.Background()
	tx, err := repo.BeginTx(ctx)
	assert.NoError(t, err)
	defer func() { assert.NoError(t, repo.RollbackTx(tx)) }()

	v := random.New(random.CryptoSeed{}).Make(testent.Foo{}).(testent.Foo)
	crudtest.Create[testent.Foo, testent.FooID](t, repo, tx, &v)
	crudtest.IsPresent[testent.Foo, testent.FooID](t, repo, tx, v.ID)
	crudtest.IsAbsent[testent.Foo, testent.FooID](t, repo, ctx, v.ID)
}

func Test_pgxTx(t *testing.T) {
	var (
		ctx   = context.Background()
		id    = "a-uniq-id"
		count int
	)

	c, err := pgxpool.New(ctx, DatabaseDSN(t))
	assert.NoError(t, err)
	defer c.Close()

	_, err = c.Exec(ctx, `CREATE TABLE IF NOT EXISTS "pgx_tx_test" ("id" TEXT NOT NULL PRIMARY KEY, "foo" TEXT NOT	NULL);`)
	assert.NoError(t, err)

	defer func() {
		_, err = c.Exec(ctx, `DROP TABLE IF EXISTS "pgx_tx_test";`)
		assert.NoError(t, err)
	}()

	assert.NoError(t, c.QueryRow(ctx, `SELECT COUNT(*) FROM pgx_tx_test WHERE id = $1`, id).Scan(&count))
	assert.Equal(t, 0, count)

	tx1, err := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	assert.NoError(t, err)
	defer tx1.Rollback(ctx)

	tag, err := tx1.Exec(ctx, `INSERT INTO pgx_tx_test (id, foo) VALUES ($1, $2)`, id, "foo/bar/baz")
	assert.NoError(t, err)
	assert.Equal(t, 1, tag.RowsAffected())

	tx2, err := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	assert.NoError(t, err)
	defer tx2.Rollback(ctx)

	assert.NoError(t, tx2.QueryRow(ctx, `SELECT COUNT(*) FROM pgx_tx_test WHERE id = $1`, id).Scan(&count))
	assert.Equal(t, 0, count)

	assert.NoError(t, c.QueryRow(ctx, `SELECT COUNT(*) FROM pgx_tx_test WHERE id = $1`, id).Scan(&count))
	assert.Equal(t, 0, count)
}

func Test_pgxQuery(t *testing.T) {
	MigrateFoo(t, GetConnection(t))

	var (
		ctx = context.Background()
		rnd = random.New(random.CryptoSeed{})
	)

	conn, err := pgxpool.New(ctx, DatabaseDSN(t))
	assert.NoError(t, err)
	defer conn.Close()

	tx, err := conn.Begin(ctx)
	assert.NoError(t, err)
	defer tx.Rollback(ctx)

	tag, err := conn.Exec(ctx, `INSERT INTO "foos" (id, foo, bar, baz) VALUES ($1, $2, $3, $4), ($5, $6, $7, $8), ($9, $10, $11, $12)`,
		rnd.UUID(), rnd.String(), rnd.String(), rnd.String(),
		rnd.UUID(), rnd.String(), rnd.String(), rnd.String(),
		rnd.UUID(), rnd.String(), rnd.String(), rnd.String())
	assert.NoError(t, err)
	assert.Equal(t, 3, tag.RowsAffected())

	rows, err := tx.Query(ctx, `SELECT id FROM "foos"`)
	assert.NoError(t, err)

	var n int
	for rows.Next() {
		n++

		var id string
		assert.NoError(t, rows.Scan(&id))
		assert.NotEmpty(t, id)

		tx.QueryRow(ctx, `SELECT FROM test_pgx_query WHERE id = $1`, id)
		assert.NoError(t, err)
	}
	rows.Close()
	assert.NoError(t, rows.Err())
	assert.Equal(t, 3, n)
}
