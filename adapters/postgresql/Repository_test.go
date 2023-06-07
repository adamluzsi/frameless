package postgresql_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"
	"github.com/adamluzsi/frameless/pkg/cache"
	"github.com/adamluzsi/frameless/pkg/cache/cachecontracts"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/frameless/ports/crud/crudtest"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/spechelper/testent"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

func TestRepository(t *testing.T) {
	mapping := spechelper.TestEntityMapping()

	cm, err := postgresql.NewConnectionManager(spechelper.DatabaseURL(t))
	assert.NoError(t, err)

	subject := &postgresql.Repository[spechelper.TestEntity, string]{
		CM:      cm,
		Mapping: mapping,
	}

	spechelper.MigrateTestEntity(t, cm)

	testcase.RunSuite(t,
		crudcontracts.Creator[spechelper.TestEntity, string](func(tb testing.TB) crudcontracts.CreatorSubject[spechelper.TestEntity, string] {
			return crudcontracts.CreatorSubject[spechelper.TestEntity, string]{
				Resource:        subject,
				MakeContext:     context.Background,
				MakeEntity:      spechelper.MakeTestEntityFunc(tb),
				SupportIDReuse:  true,
				SupportRecreate: true,
			}
		}),
		crudcontracts.Finder[spechelper.TestEntity, string](func(tb testing.TB) crudcontracts.FinderSubject[spechelper.TestEntity, string] {
			return crudcontracts.FinderSubject[spechelper.TestEntity, string]{
				Resource:    subject,
				MakeContext: context.Background,
				MakeEntity:  spechelper.MakeTestEntityFunc(tb),
			}
		}),
		crudcontracts.Updater[spechelper.TestEntity, string](func(tb testing.TB) crudcontracts.UpdaterSubject[spechelper.TestEntity, string] {
			return crudcontracts.UpdaterSubject[spechelper.TestEntity, string]{
				Resource:     subject,
				MakeContext:  context.Background,
				MakeEntity:   spechelper.MakeTestEntityFunc(tb),
				ChangeEntity: nil, // test entity can be freely changed
			}
		}),
		crudcontracts.Deleter[spechelper.TestEntity, string](func(tb testing.TB) crudcontracts.DeleterSubject[spechelper.TestEntity, string] {
			return crudcontracts.DeleterSubject[spechelper.TestEntity, string]{
				Resource:    subject,
				MakeContext: context.Background,
				MakeEntity:  spechelper.MakeTestEntityFunc(tb),
			}
		}),
		crudcontracts.OnePhaseCommitProtocol[spechelper.TestEntity, string](func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[spechelper.TestEntity, string] {
			return crudcontracts.OnePhaseCommitProtocolSubject[spechelper.TestEntity, string]{
				Resource:      subject,
				CommitManager: subject.CM,
				MakeContext:   context.Background,
				MakeEntity:    spechelper.MakeTestEntityFunc(tb),
			}
		}),
	)
}

func TestRepository_mappingHasSchemaInTableName(t *testing.T) {
	cm := GetConnectionManager(t)
	spechelper.MigrateTestEntity(t, cm)

	mapper := spechelper.TestEntityMapping()
	mapper.Table = `public.` + mapper.Table

	subject := postgresql.Repository[spechelper.TestEntity, string]{
		Mapping: mapper,
		CM:      cm,
	}

	testcase.RunSuite(t, crudcontracts.Creator[spechelper.TestEntity, string](func(tb testing.TB) crudcontracts.CreatorSubject[spechelper.TestEntity, string] {
		return crudcontracts.CreatorSubject[spechelper.TestEntity, string]{
			Resource:        subject,
			MakeContext:     context.Background,
			MakeEntity:      spechelper.MakeTestEntityFunc(tb),
			SupportIDReuse:  true,
			SupportRecreate: true,
		}
	}))
}

func TestRepository_implementsCacheEntityRepository(t *testing.T) {
	cm := GetConnectionManager(t)
	spechelper.MigrateTestEntity(t, cm)

	cachecontracts.EntityRepository[spechelper.TestEntity, string](func(tb testing.TB) cachecontracts.EntityRepositorySubject[spechelper.TestEntity, string] {
		repo := postgresql.Repository[spechelper.TestEntity, string]{
			Mapping: spechelper.TestEntityMapping(),
			CM:      cm,
		}
		return cachecontracts.EntityRepositorySubject[spechelper.TestEntity, string]{
			EntityRepository: repo,
			CommitManager:    cm,
			MakeContext:      context.Background,
			MakeEntity:       spechelper.MakeTestEntityFunc(tb),
			ChangeEntity:     nil,
		}
	}).Test(t)
}

func TestRepository_canImplementCacheHitRepository(t *testing.T) {
	cm := GetConnectionManager(t)

	func(tb testing.TB, cm postgresql.ConnectionManager) {
		const testCacheHitMigrateUP = `CREATE TABLE "test_cache_hits" ( id TEXT PRIMARY KEY, ids TEXT[], ts TIMESTAMP WITH TIME ZONE );`
		const testCacheHitMigrateDOWN = `DROP TABLE IF EXISTS "test_cache_hits";`

		ctx := context.Background()
		c, err := cm.Connection(ctx)
		assert.Nil(tb, err)
		_, err = c.ExecContext(ctx, testCacheHitMigrateDOWN)
		assert.Nil(tb, err)
		_, err = c.ExecContext(ctx, testCacheHitMigrateUP)
		assert.Nil(tb, err)

		tb.Cleanup(func() {
			client, err := cm.Connection(ctx)
			assert.Nil(tb, err)
			_, err = client.ExecContext(ctx, testCacheHitMigrateDOWN)
			assert.Nil(tb, err)
		})
	}(t, cm)

	hitRepo := postgresql.Repository[cache.Hit[string], cache.HitID]{
		Mapping: postgresql.Mapper[cache.Hit[string], cache.HitID]{
			Table:   "test_cache_hits",
			ID:      "id",
			Columns: []string{"id", "ids", "ts"},
			ToArgsFn: func(ptr *cache.Hit[string]) ([]interface{}, error) {
				return []any{ptr.QueryID, &ptr.EntityIDs, ptr.Timestamp}, nil
			},
			MapFn: func(scanner iterators.SQLRowScanner) (cache.Hit[string], error) {
				var hit cache.Hit[string]
				if err := scanner.Scan(&hit.QueryID, &hit.EntityIDs, &hit.Timestamp); err != nil {
					return hit, err
				}
				hit.Timestamp = hit.Timestamp.UTC()
				return hit, nil
			},
		},
		CM: cm,
	}

	cachecontracts.HitRepository[string](func(tb testing.TB) cachecontracts.HitRepositorySubject[string] {
		return cachecontracts.HitRepositorySubject[string]{
			Resource:      hitRepo,
			CommitManager: cm,
			MakeContext:   context.Background,
			MakeHit: func() cache.Hit[string] {
				t := tb.(*testcase.T)
				return cache.Hit[string]{
					QueryID: t.Random.UUID(),
					EntityIDs: random.Slice(t.Random.IntBetween(0, 7), func() string {
						return t.Random.UUID()
					}),
					Timestamp: t.Random.Time().UTC(),
				}
			},
		}
	}).Test(t)
}

func TestRepository_comprotoOnePhaseCommitProtocol(t *testing.T) {
	repo := &postgresql.Repository[testent.Foo, testent.FooID]{
		CM:      GetConnectionManager(t),
		Mapping: FooMapping,
	}
	MigrateFoo(t, repo.CM)

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

	c, err := pgxpool.New(ctx, spechelper.DatabaseDSN(t))
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
	MigrateFoo(t, GetConnectionManager(t))

	var (
		ctx = context.Background()
		rnd = random.New(random.CryptoSeed{})
	)

	conn, err := pgxpool.New(ctx, spechelper.DatabaseDSN(t))
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
