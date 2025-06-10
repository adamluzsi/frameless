package mariadb_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/mariadb"
	"go.llib.dev/frameless/adapter/mariadb/internal/queries"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/tasker/taskercontracts"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/guard/guardcontracts"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/migration/migrationcontracts"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestRepository(t *testing.T) {
	logger.Testing(t)
	cm := GetConnection(t)

	subject := &mariadb.Repository[Entity, EntityID]{
		Connection: cm,
		Mapping:    EntityMapping(),
	}

	MigrateEntity(t, cm)

	config := crudcontracts.Config[Entity, EntityID]{
		MakeContext:     func(t testing.TB) context.Context { return context.Background() },
		SupportIDReuse:  true,
		SupportRecreate: true,
		ChangeEntity:    nil, // test entity can be freely changed
	}

	testcase.RunSuite(t,
		crudcontracts.Creator[Entity, EntityID](subject, config),
		crudcontracts.Finder[Entity, EntityID](subject, config),
		crudcontracts.ByIDsFinder[Entity, EntityID](subject, config),
		crudcontracts.Updater[Entity, EntityID](subject, config),
		crudcontracts.Saver[Entity, EntityID](subject, config),
		crudcontracts.Deleter[Entity, EntityID](subject, config),
		crudcontracts.OnePhaseCommitProtocol[Entity, EntityID](subject, subject.Connection),
	)
}

func TestCacheRepository(t *testing.T) {
	logger.Testing(t)
	ctx := context.Background()
	cm := GetConnection(t)

	subject := mariadb.CacheRepository[testent.Foo, testent.FooID]{
		Connection: GetConnection(t),
		ID:         "foo",
		JSONDTOM:   testent.FooJSONMapping(),
		IDA: func(f *testent.Foo) *testent.FooID {
			return &f.ID
		},
		IDM: dtokit.Mapping[testent.FooID, string]{
			ToENT: func(ctx context.Context, dto string) (testent.FooID, error) {
				return testent.FooID(dto), nil
			},
			ToDTO: func(ctx context.Context, ent testent.FooID) (string, error) {
				return ent.String(), nil
			},
		},
	}
	assert.NoError(t, subject.Migrate(ctx))

	conf := cachecontracts.Config[testent.Foo, testent.FooID]{
		CRUD: crudcontracts.Config[testent.Foo, testent.FooID]{
			MakeEntity: func(tb testing.TB) testent.Foo {
				foo := testent.MakeFoo(tb)
				foo.ID = testent.FooID(testcase.ToT(&tb).Random.UUID())
				return foo
			},
		},
	}

	cachecontracts.EntityRepository[testent.Foo, testent.FooID](subject.Entities(), cm, conf)
	cachecontracts.HitRepository[testent.FooID](subject.Hits(), cm)
	cachecontracts.Repository(subject, conf).Test(t)
}

func TestMigrationStateRepository(t *testing.T) {
	logger.Testing(t)
	ctx := context.Background()
	conn := GetConnection(t)

	repo := mariadb.MakeMigrationStateRepository(conn)
	repo.Mapping.TableName += "_test"

	_, err := conn.ExecContext(ctx, fmt.Sprintf(queries.CreateTableSchemaMigrationsTmpl, repo.Mapping.TableName))
	assert.NoError(t, err)
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, fmt.Sprintf(queries.DropTableTmpl, repo.Mapping.TableName)) })

	migrationcontracts.StateRepository(repo).Test(t)
}

func TestMigrationStateRepository_smoke(t *testing.T) {
	logger.Testing(t)
	ctx := context.Background()
	conn := GetConnection(t)

	repo := mariadb.MakeMigrationStateRepository(conn)
	repo.Mapping.TableName += "_test"

	_, err := conn.ExecContext(ctx, fmt.Sprintf(queries.CreateTableSchemaMigrationsTmpl, repo.Mapping.TableName))
	assert.NoError(t, err)
	t.Cleanup(func() { _, _ = conn.ExecContext(ctx, fmt.Sprintf(queries.DropTableTmpl, repo.Mapping.TableName)) })

	ent1 := migration.State{
		ID: migration.StateID{
			Namespace: "ns",
			Version:   "0",
		},
		Dirty: false,
	}

	assert.NoError(t, repo.Create(ctx, &ent1))

	gotEnt1, found, err := repo.FindByID(ctx, ent1.ID)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, gotEnt1, ent1)

	assert.NoError(t, repo.DeleteByID(ctx, ent1.ID))

	assert.NoError(t, repo.Create(ctx, &ent1))
	assert.NoError(t, repo.DeleteAll(ctx))
}

func ExampleLocker() {
	cm, err := mariadb.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	l := mariadb.Locker{
		Name:       "my-lock",
		Connection: cm,
	}

	ctx, err := l.Lock(context.Background())
	if err != nil {
		panic(err)
	}

	if err := l.Unlock(ctx); err != nil {
		panic(err)
	}
}

var _ migration.Migratable = mariadb.Locker{}

func TestLocker(t *testing.T) {
	cm := GetConnection(t)

	l := mariadb.Locker{
		Name:       rnd.StringNC(5, random.CharsetAlpha()),
		Connection: cm,
	}
	assert.NoError(t, l.Migrate(context.Background()))

	guardcontracts.Locker(l).Test(t)
}

func ExampleLockerFactory() {
	cm, err := mariadb.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	lockerFactory := mariadb.LockerFactory[string]{Connection: cm}
	if err := lockerFactory.Migrate(context.Background()); err != nil {
		log.Fatal(err)
	}

	locker := lockerFactory.LockerFor("hello world")

	ctx, err := locker.Lock(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	if err := locker.Unlock(ctx); err != nil {
		log.Fatal(err)
	}
}

var _ migration.Migratable = mariadb.LockerFactory[int]{}

func TestNewLockerFactory(t *testing.T) {
	ctx := context.Background()
	cm := GetConnection(t)

	lockerFactoryStrKey := mariadb.LockerFactory[string]{Connection: cm}
	assert.NoError(t, lockerFactoryStrKey.Migrate(ctx))
	assert.NoError(t, lockerFactoryStrKey.Purge(ctx))

	testcase.RunSuite(t,
		guardcontracts.LockerFactory[string](lockerFactoryStrKey),
		guardcontracts.NonBlockingLockerFactory[string](lockerFactoryStrKey),
	)
}

func TestNewLockerFactory_altKeyInt(t *testing.T) {
	ctx := context.Background()
	cm := GetConnection(t)

	lockerFactoryIntKey := mariadb.LockerFactory[int]{Connection: cm}
	assert.NoError(t, lockerFactoryIntKey.Migrate(ctx))

	testcase.RunSuite(t,
		guardcontracts.LockerFactory[int](lockerFactoryIntKey),
		guardcontracts.NonBlockingLockerFactory[int](lockerFactoryIntKey),
	)
}

var _ migration.Migratable = mariadb.TaskerSchedulerLocks{}
var _ migration.Migratable = mariadb.TaskerSchedulerStateRepository{}

func TestTaskerSchedulerStateRepository(t *testing.T) {
	cm := GetConnection(t)

	r := mariadb.TaskerSchedulerStateRepository{Connection: cm}
	assert.NoError(t, r.Migrate(context.Background()))
	taskercontracts.ScheduleStateRepository(r).Test(t)
}

func TestTaskerSchedulerLocks(t *testing.T) {
	cm := GetConnection(t)

	l := mariadb.TaskerSchedulerLocks{Connection: cm}
	assert.NoError(t, l.Migrate(context.Background()))
	taskercontracts.SchedulerLocks(l).Test(t)
}

func TestLockerFactory_Namespace_smoke(t *testing.T) {
	logger.Testing(t)
	cm := GetConnection(t)

	lf1 := mariadb.LockerFactory[string]{
		Connection: cm,
		Namespace:  "1",
	}

	lf2 := mariadb.LockerFactory[string]{
		Connection: cm,
		Namespace:  "2",
	}

	const key = "lock-name"

	locker1 := lf1.LockerFor(key)
	locker2 := lf2.LockerFor(key)

	ctx := context.Background()

	assert.Within(t, time.Second, func(context.Context) {
		lctx1, err := locker1.Lock(ctx)
		assert.NoError(t, err)
		t.Cleanup(func() { locker1.Unlock(lctx1) })
	})

	assert.Within(t, time.Second, func(context.Context) {
		lctx2, err := locker2.Lock(ctx)
		assert.NoError(t, err)
		defer locker2.Unlock(lctx2)
	})
}
