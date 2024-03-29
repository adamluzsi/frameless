package postgresql_test

import (
	"context"
	"log"
	"os"
	"testing"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/adapters/postgresql"
	"go.llib.dev/frameless/ports/guard/guardcontracts"
	"go.llib.dev/frameless/ports/migration"
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

	guardcontracts.Locker(func(tb testing.TB) guardcontracts.LockerSubject {
		t := testcase.ToT(&tb)
		l := postgresql.Locker{
			Name:       t.Random.StringNC(5, random.CharsetAlpha()),
			Connection: cm,
		}
		assert.NoError(tb, l.Migrate(context.Background()))
		return guardcontracts.LockerSubject{
			Locker:      l,
			MakeContext: context.Background,
		}
	}).Test(t)
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
	cm := GetConnection(t)

	guardcontracts.LockerFactory[string](func(tb testing.TB) guardcontracts.LockerFactorySubject[string] {
		lockerFactory := postgresql.LockerFactory[string]{Connection: cm}
		assert.NoError(tb, lockerFactory.Migrate(context.Background()))
		return guardcontracts.LockerFactorySubject[string]{
			LockerFactory: lockerFactory,
			MakeContext:   context.Background,
			MakeKey:       testcase.ToT(&tb).Random.String,
		}
	}).Test(t)

	guardcontracts.LockerFactory[int](func(tb testing.TB) guardcontracts.LockerFactorySubject[int] {
		lockerFactory := postgresql.LockerFactory[int]{Connection: cm}
		assert.NoError(tb, lockerFactory.Migrate(context.Background()))
		return guardcontracts.LockerFactorySubject[int]{
			LockerFactory: lockerFactory,
			MakeContext:   context.Background,
			MakeKey:       testcase.ToT(&tb).Random.Int,
		}
	}).Test(t)
}
