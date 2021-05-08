package postgresql_test

import (
	"context"
	"github.com/adamluzsi/frameless/resources/postgresql"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/resources/contracts"
	"github.com/adamluzsi/testcase"
)

type StorageTestEntity struct {
	ID  string `ext:"ID"`
	Foo string
	Bar string
	Baz string
}

func TestEntityStorage(t *testing.T) {
	postgresql.WithDebug(t)

	pool := &postgresql.DefaultPool{DSN: GetDatabaseURL(t)}

	subject := &postgresql.Storage{
		Pool: pool,
		Mapping: postgresql.Mapper{
			Table:   "storage_test_entities",
			ID:      "id",
			Columns: []string{`id`, `foo`, `bar`, `baz`},
			NewIDFn: func(ctx context.Context) (interface{}, error) {
				return fixtures.Random.StringN(42), nil
			},
			ToArgsFn: func(ptr interface{}) ([]interface{}, error) {
				ent := ptr.(*StorageTestEntity)
				return []interface{}{ent.ID, ent.Foo, ent.Bar, ent.Baz}, nil
			},
			MapFn: func(s iterators.SQLRowScanner, ptr interface{}) error {
				ent := ptr.(*StorageTestEntity)
				return s.Scan(&ent.ID, &ent.Foo, &ent.Bar, &ent.Baz)
			},
		},
	}

	migrateEntityStorage(t, pool)

	T := StorageTestEntity{}
	ff := fixtures.FixtureFactory{}
	testcase.RunContract(t,
		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: ff},
		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) contracts.OnePhaseCommitProtocolSubject { return subject }, FixtureFactory: ff},
		contracts.CreatorPublisher{T: T, Subject: func(tb testing.TB) contracts.CreatorPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.UpdaterPublisher{T: T, Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.DeleterPublisher{T: T, Subject: func(tb testing.TB) contracts.DeleterPublisherSubject { return subject }, FixtureFactory: ff},
	)
}

func migrateEntityStorage(tb testing.TB, pool *postgresql.DefaultPool) {
	ctx := context.Background()
	client, free, err := pool.GetClient(ctx)
	require.Nil(tb, err)
	defer free()
	_, err = client.ExecContext(ctx, storageTestMigrateDOWN)
	require.Nil(tb, err)
	_, err = client.ExecContext(ctx, storageTestMigrateUP)
	require.Nil(tb, err)

	tb.Cleanup(func() {
		client, free, err := pool.GetClient(ctx)
		require.Nil(tb, err)
		defer free()
		_, err = client.ExecContext(ctx, storageTestMigrateDOWN)
		require.Nil(tb, err)
	})
}

const storageTestMigrateUP = `
CREATE TABLE "storage_test_entities" (
    id	TEXT	NOT	NULL	PRIMARY KEY,
	foo	TEXT	NOT	NULL,
	bar	TEXT	NOT	NULL,
	baz	TEXT	NOT	NULL
);
`

const storageTestMigrateDOWN = `
DROP TABLE IF EXISTS "storage_test_entities";
`
