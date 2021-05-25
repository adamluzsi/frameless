package postgresql_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/postgresql"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase"
)

type StorageTestEntity struct {
	ID  string `ext:"ID"`
	Foo string
	Bar string
	Baz string
}

func StorageTestEntityMapping() postgresql.Mapper {
	return postgresql.Mapper{
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
	}
}

func TestNewStorage_smokeTest(t *testing.T) {
	cm := &postgresql.ConnectionManager{DSN: GetDatabaseURL(t)}
	defer cm.Close()
	migrateEntityStorage(t, cm)

	storage := &postgresql.Storage{
		T:                 StorageTestEntity{},
		ConnectionManager: cm,
		Mapping:           StorageTestEntityMapping(),
	}

	ctx := context.Background()

	ent := &StorageTestEntity{
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	require.NoError(t, storage.Create(ctx, ent))
	require.NotEmpty(t, ent.ID)

	var ent2 StorageTestEntity
	found, err := storage.FindByID(ctx, &ent2, ent.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, *ent, ent2)

	require.NoError(t, storage.DeleteByID(ctx, ent.ID))
	found, err = storage.FindByID(ctx, &StorageTestEntity{}, ent.ID)
	require.NoError(t, err)
	require.False(t, found, `should be deleted`)
}

func TestStorage(t *testing.T) {
	T := StorageTestEntity{}
	ff := fixtures.FixtureFactory{}
	cm := &postgresql.ConnectionManager{DSN: GetDatabaseURL(t)}

	subject := &postgresql.Storage{
		T:                 T,
		ConnectionManager: cm,
		Mapping:           StorageTestEntityMapping(),
	}

	migrateEntityStorage(t, cm)

	testcase.RunContract(t,
		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: ff},
		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) { return cm, subject }, FixtureFactory: ff},
		contracts.CreatorPublisher{T: T, Subject: func(tb testing.TB) contracts.CreatorPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.UpdaterPublisher{T: T, Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.DeleterPublisher{T: T, Subject: func(tb testing.TB) contracts.DeleterPublisherSubject { return subject }, FixtureFactory: ff},
	)
}

func TestStorage_mappingHasSchemaInTableName(t *testing.T) {
	T := StorageTestEntity{}
	ff := fixtures.FixtureFactory{}
	cm := &postgresql.ConnectionManager{DSN: GetDatabaseURL(t)}
	migrateEntityStorage(t, cm)

	mapper := StorageTestEntityMapping()
	mapper.Table = `public.` + mapper.Table

	subject := &postgresql.Storage{
		T:                 StorageTestEntity{},
		ConnectionManager: cm,
		Mapping:           mapper,
	}

	testcase.RunContract(t,
		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: ff},
		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: ff},
		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) { return cm, subject }, FixtureFactory: ff},
		contracts.CreatorPublisher{T: T, Subject: func(tb testing.TB) contracts.CreatorPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.UpdaterPublisher{T: T, Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject { return subject }, FixtureFactory: ff},
		contracts.DeleterPublisher{T: T, Subject: func(tb testing.TB) contracts.DeleterPublisherSubject { return subject }, FixtureFactory: ff},
	)
}

func migrateEntityStorage(tb testing.TB, cm *postgresql.ConnectionManager) {
	ctx := context.Background()
	c, err := cm.GetConnection(ctx)
	require.Nil(tb, err)
	_, err = c.ExecContext(ctx, storageTestMigrateUP)
	require.Nil(tb, err)

	tb.Cleanup(func() {
		client, err := cm.GetConnection(ctx)
		require.Nil(tb, err)
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
