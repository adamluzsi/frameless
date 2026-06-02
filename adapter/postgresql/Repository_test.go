package postgresql_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

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
	"go.llib.dev/testcase/let"
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
	s := testcase.NewSpec(t)
	mapping := EntityMapping()

	cm, err := postgresql.Connect(DatabaseURL(t))
	assert.NoError(t, err)

	MigrateEntity(t, cm)

	subject := &postgresql.Repository[Entity, string]{
		Connection: cm,
		Mapping:    mapping,
	}

	config := crudcontract.Config[Entity, string]{
		MakeContext: func(t testing.TB) context.Context {
			return context.Background()
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
		ChangeEntity:    nil, // test entity can be freely changed
		OnePhaseCommit:  cm,
	}

	testcase.RunSuite(s,
		crudcontract.Creator[Entity, string](subject, config),
		crudcontract.Finder[Entity, string](subject, config),
		crudcontract.Updater[Entity, string](subject, config),
		crudcontract.Deleter[Entity, string](subject, config),
		crudcontract.OnePhaseCommitProtocol[Entity, string](subject, subject.Connection),
		crudcontract.Batcher(subject, config))
}

func TestRepository_Truncate(t *testing.T) {
	cm, err := postgresql.Connect(DatabaseURL(t))
	assert.NoError(t, err)

	MigrateEntity(t, cm)

	repo := postgresql.Repository[Entity, string]{
		Connection: cm,
		Mapping:    EntityMapping(),
	}

	ctx := context.Background()
	makeEntity := MakeEntityFunc(t)

	// createEntities creates n entities in the resource and returns their IDs.
	createEntities := func(t *testing.T, n int) []string {
		var ids []string
		for i := 0; i < n; i++ {
			ent := makeEntity()
			crudtest.Create[Entity, string](t, repo, ctx, &ent)
			ids = append(ids, ent.ID)
		}
		return ids
	}

	// always leave a clean table behind for the next test case.
	t.Cleanup(func() { assert.NoError(t, repo.Truncate(ctx)) })

	t.Run("Truncate removes all created entities", func(t *testing.T) {
		assert.NoError(t, repo.Truncate(ctx))

		ids := createEntities(t, 3)

		assert.NoError(t, repo.Truncate(ctx))

		for _, id := range ids {
			crudtest.IsAbsent[Entity, string](t, repo, ctx, id)
		}
	})

	t.Run("Truncate within a committed transaction removes all created entities", func(t *testing.T) {
		assert.NoError(t, repo.Truncate(ctx))

		ids := createEntities(t, 3)

		tx, err := repo.BeginTx(ctx)
		assert.NoError(t, err)

		assert.NoError(t, repo.Truncate(tx))
		assert.NoError(t, repo.CommitTx(tx))

		for _, id := range ids {
			crudtest.IsAbsent[Entity, string](t, repo, ctx, id)
		}
	})

	t.Run("Truncate within a rolled back transaction leaves the created entities intact", func(t *testing.T) {
		assert.NoError(t, repo.Truncate(ctx))

		ids := createEntities(t, 3)

		tx, err := repo.BeginTx(ctx)
		assert.NoError(t, err)

		assert.NoError(t, repo.Truncate(tx))
		assert.NoError(t, repo.RollbackTx(tx))

		for _, id := range ids {
			crudtest.IsPresent[Entity, string](t, repo, ctx, id)
		}
	})
}

func TestRepository_BatchConfig(t *testing.T) {
	s := testcase.NewSpec(t)
	mapping := EntityMapping()

	cm, err := postgresql.Connect(DatabaseURL(t))
	assert.NoError(t, err)

	MigrateEntity(t, cm)

	baseConfig := crudcontract.Config[Entity, string]{
		MakeContext: func(t testing.TB) context.Context {
			return context.Background()
		},
		SupportIDReuse:  true,
		SupportRecreate: true,
		ChangeEntity:    nil, // test entity can be freely changed
		OnePhaseCommit:  cm,
	}

	s.Context("no transaction", func(s *testcase.Spec) {
		subject := &postgresql.Repository[Entity, string]{
			Connection: cm,
			Mapping:    mapping,
			BatchConfig: postgresql.BatchConfig{
				NoTransaction: true,
			},
		}
		config := baseConfig
		config.OnePhaseCommit = nil
		crudcontract.Batcher(subject, config).Spec(s)
	})

	s.Context("with staging table", func(s *testcase.Spec) {
		subject := &postgresql.Repository[Entity, string]{
			Connection: cm,
			Mapping:    mapping,
			BatchConfig: postgresql.BatchConfig{
				UseStagingTable: true,
			},
		}
		config := baseConfig
		crudcontract.Batcher(subject, config).Spec(s)

		s.Context("without transaction", func(s *testcase.Spec) {
			subject := &postgresql.Repository[Entity, string]{
				Connection: cm,
				Mapping:    mapping,
				BatchConfig: postgresql.BatchConfig{
					UseStagingTable: true,
					NoTransaction:   true,
				},
			}
			config := baseConfig
			config.OnePhaseCommit = nil

			s.After(func(t *testcase.T) {
				query := `SELECT tablename FROM pg_tables WHERE schemaname = 'pg_temp' AND tablename ILIKE '%staging%';`
				rows, err := cm.QueryContext(t.Context(), query)
				assert.NoError(t, err)

				for rows.Next() {
					var tableName string
					assert.NoError(t, rows.Scan(&tableName))
					t.Errorf("unexpected staging table found: %s", tableName)
					break
				}
				assert.NoError(t, rows.Err())
				assert.NoError(t, rows.Close())
			})

			crudcontract.Batcher(subject, config).Spec(s)
		})
	})
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

	crudcontract.Creator(subject, crudcontract.Config[Entity, string]{
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

func TestRepository_implementsCRUDBatcher(t *testing.T) {
	cm := GetConnection(t)
	MigrateEntity(t, cm)

	repo := postgresql.Repository[Entity, string]{
		Mapping:    EntityMapping(),
		Connection: cm,
	}

	crudcontract.Batcher(repo, crudcontract.Config[Entity, string]{
		OnePhaseCommit: cm,
	}).Test(t)

	time.Sleep(time.Second)
}

func TestConnection_nRollbackTx(tt *testing.T) {
	t := testcase.NewT(tt)

	cm := GetConnection(tt)
	MigrateEntity(tt, cm)

	ctx := context.Background()
	ctx, err := cm.BeginTx(ctx)
	assert.NoError(tt, err)

	assert.NoError(tt, cm.RollbackTx(ctx))
	t.Random.Repeat(3, 7, func() {
		cm.RollbackTx(ctx)
	})
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

func TestBatch_rainyAdd(t *testing.T) {
	cm := GetConnection(t)
	MigrateEntity(t, cm)

	mapping := EntityMapping()
	mapping.TableName = "incorrect_non_existent"

	subject := &postgresql.Repository[Entity, string]{
		Connection: cm,
		Mapping:    mapping,
	}

	batch := subject.Batch(t.Context())
	defer batch.Close()

	assert.Within(t, time.Second, func(ctx context.Context) {
		assert.AnyOf(t, func(a *assert.A) {
			a.Test(func(t testing.TB) { assert.Error(t, batch.Add(MakeEntityFunc(t)())) })
			a.Test(func(t testing.TB) { assert.Error(t, batch.Close()) })
		})
	})
}

func Benchmark_batch(b *testing.B) {
	if testing.Short() {
		b.Skip("testing.Short()")
	}

	s := testcase.NewSpec(b)

	var (
		cm   = GetConnection(b)
		ctx  = context.Background()
		repo = &postgresql.Repository[Entity, string]{
			Connection: cm,
			Mapping:    EntityMapping(),
		}
	)

	s.Before(func(t *testcase.T) {
		MigrateEntity(t, cm)
		MigrateEntityIndexes(t, cm)
	})

	var samplings = []int{
		100,
		1000,
		10000,
		100000,
		1000000,
	}

	rnd := random.New(random.CryptoSeed{})
	var makeEntities = func(t *testcase.T, n int) []Entity {
		return random.Slice(n, func() Entity {
			return Entity{
				ID:  rnd.UUID(),
				Foo: rnd.UUID(),
				Bar: rnd.UUID(),
				Baz: rnd.UUID(),
			}
		}, random.UniqueValues)
	}

	s.Before(func(t *testcase.T) {
		_, err := cm.ExecContext(ctx, "TRUNCATE TABLE test_entities")
		assert.NoError(t, err)
	})

	var cases = func(s *testcase.Spec, n int) {
		entities := let.Var(s, func(t *testcase.T) []Entity {
			return makeEntities(t, n)
		})

		s.Benchmark("Batch", func(t *testcase.T) {
			batch := repo.Batch(ctx)
			for _, e := range entities.Get(t) {
				assert.NoError(b, batch.Add(e))
			}
			assert.NoError(b, batch.Close())
		})

		s.Benchmark("Tx+COPY", func(t *testcase.T) {
			ents := entities.Get(t)
			tx, err := cm.DB.Begin(ctx)
			assert.NoError(b, err)

			// Prepare COPY data
			rows := make([][]any, len(ents))
			for i, e := range ents {
				rows[i] = []any{e.ID, e.Foo, e.Bar, e.Baz}
			}

			_, err = tx.CopyFrom(
				ctx,
				pgx.Identifier{"test_entities"},
				[]string{"id", "foo", "bar", "baz"},
				pgx.CopyFromRows(rows),
			)
			assert.NoError(b, err)

			err = tx.Commit(ctx)
			assert.NoError(b, err)
		})
	}

	for _, sampling := range samplings {
		n := sampling

		s.Context(fmt.Sprintf("%d", n), func(s *testcase.Spec) {
			s.Context("empty", func(s *testcase.Spec) {
				cases(s, n)
			})

			s.Context("prefilled", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					batch := repo.Batch(t.Context())
					for _, e := range makeEntities(t, 1_000_000) {
						assert.NoError(b, batch.Add(e))
					}
					assert.NoError(b, batch.Close())
				})

				cases(s, n)
			})
		})
	}
}
