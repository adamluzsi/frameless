package postgresql_test

import (
	"context"
	"database/sql"
	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"
	"github.com/adamluzsi/frameless/pkg/cache"
	"github.com/adamluzsi/frameless/pkg/cache/cachecontracts"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/testcase/random"
	"github.com/lib/pq"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

func Test_postgresConnection(t *testing.T) {
	it := assert.MakeIt(t)

	t.Log(spechelper.DatabaseURL(t))

	db, err := sql.Open("postgres", spechelper.DatabaseURL(t))
	it.Must.Nil(err)
	it.Must.Nil(db.Ping())

	var b bool
	it.Must.Nil(db.QueryRow("SELECT TRUE").Scan(&b))
	it.Must.True(b)
}

func TestRepository(t *testing.T) {
	mapping := spechelper.TestEntityMapping()
	db, err := sql.Open("postgres", spechelper.DatabaseURL(t))
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	cm, err := postgresql.NewConnectionManagerWithDSN(spechelper.DatabaseURL(t))
	assert.NoError(t, err)

	subject := &postgresql.Repository[spechelper.TestEntity, string]{
		ConnectionManager: cm,
		Mapping:           mapping,
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
				CommitManager: subject.ConnectionManager,
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
		Mapping:           mapper,
		ConnectionManager: cm,
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
			Mapping:           spechelper.TestEntityMapping(),
			ConnectionManager: cm,
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
				return []any{ptr.QueryID, pq.Array(ptr.EntityIDs), ptr.Timestamp}, nil
			},
			MapFn: func(scanner iterators.SQLRowScanner) (cache.Hit[string], error) {
				var hit cache.Hit[string]
				if err := scanner.Scan(&hit.QueryID, pq.Array(&hit.EntityIDs), &hit.Timestamp); err != nil {
					return hit, err
				}
				hit.Timestamp = hit.Timestamp.UTC()
				return hit, nil
			},
		},
		ConnectionManager: cm,
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
