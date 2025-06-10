package postgresql_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/port/guard/guardcontracts"
	"go.llib.dev/frameless/port/migration"
)

func ExampleLocker() {
	cm, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	l := postgresql.Locker{
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

var _ migration.Migratable = postgresql.Locker{}

func TestLocker(t *testing.T) {
	cm := GetConnection(t)

	l := postgresql.Locker{
		Name:       rnd.StringNC(5, random.CharsetAlpha()),
		Connection: cm,
	}
	assert.NoError(t, l.Migrate(context.Background()))

	testcase.RunSuite(t,
		guardcontracts.Locker(l),
		guardcontracts.NonBlockingLocker(l),
	)
}

func ExampleLockerFactory() {
	cm, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	lockerFactory := postgresql.LockerFactory[string]{Connection: cm}
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

var _ migration.Migratable = postgresql.LockerFactory[int]{}

func TestNewLockerFactory(t *testing.T) {
	ctx := context.Background()
	cm := GetConnection(t)

	lockerFactoryStrKey := postgresql.LockerFactory[string]{Connection: cm}
	assert.NoError(t, lockerFactoryStrKey.Migrate(ctx))
	guardcontracts.LockerFactory[string](lockerFactoryStrKey).Test(t)

	lockerFactoryIntKey := postgresql.LockerFactory[int]{Connection: cm}
	assert.NoError(t, lockerFactoryIntKey.Migrate(ctx))
	guardcontracts.LockerFactory[int](lockerFactoryIntKey).Test(t)
}

func TestLocker_TryLock_smoke(t *testing.T) {
	const timeout = 3 * time.Second
	c := GetConnection(t)
	l := postgresql.Locker{Name: rnd.Domain(), Connection: c}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, l.Migrate(ctx))

	var lockContext context.Context

	assert.Within(t, timeout, func(context.Context) {
		var acquired bool
		var err error
		lockContext, acquired, err = l.TryLock(ctx)
		assert.NoError(t, err)
		assert.True(t, acquired)
		assert.NotNil(t, lockContext)
		t.Cleanup(func() { l.Unlock(lockContext) })
	})

	assert.NotNil(t, lockContext)

	assert.Within(t, timeout, func(wictx context.Context) {
		lctx, acquired, err := l.TryLock(wictx)
		assert.NoError(t, err)
		assert.False(t, acquired)
		assert.Nil(t, lctx)
	})
}
