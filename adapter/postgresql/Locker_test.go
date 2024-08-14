package postgresql_test

import (
	"context"
	"log"
	"os"
	"testing"

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

	guardcontracts.Locker(l).Test(t)
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
