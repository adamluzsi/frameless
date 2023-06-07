package postgresql_test

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/ports/locks/lockscontracts"
	"github.com/adamluzsi/frameless/ports/migration"
)

func ExampleLocker() {
	cm, err := postgresql.NewConnectionManager(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	l := postgresql.Locker{
		Name: "my-lock",
		CM:   cm,
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
	cm := GetConnectionManager(t)

	lockscontracts.Locker(func(tb testing.TB) lockscontracts.LockerSubject {
		t := testcase.ToT(&tb)
		l := postgresql.Locker{
			Name: t.Random.StringNC(5, random.CharsetAlpha()),
			CM:   cm,
		}
		assert.NoError(tb, l.Migrate(context.Background()))
		return lockscontracts.LockerSubject{
			Locker:      l,
			MakeContext: context.Background,
		}
	}).Test(t)
}

func ExampleLockerFactory() {
	cm, err := postgresql.NewConnectionManager(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	lockerFactory := postgresql.LockerFactory[string]{CM: cm}
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
	cm := GetConnectionManager(t)

	lockscontracts.Factory[string](func(tb testing.TB) lockscontracts.FactorySubject[string] {
		lockerFactory := postgresql.LockerFactory[string]{CM: cm}
		assert.NoError(tb, lockerFactory.Migrate(context.Background()))
		return lockscontracts.FactorySubject[string]{
			Factory:     lockerFactory,
			MakeContext: context.Background,
			MakeKey:     testcase.ToT(&tb).Random.String,
		}
	}).Test(t)

	lockscontracts.Factory[int](func(tb testing.TB) lockscontracts.FactorySubject[int] {
		lockerFactory := postgresql.LockerFactory[int]{CM: cm}
		assert.NoError(tb, lockerFactory.Migrate(context.Background()))
		return lockscontracts.FactorySubject[int]{
			Factory:     lockerFactory,
			MakeContext: context.Background,
			MakeKey:     testcase.ToT(&tb).Random.Int,
		}
	}).Test(t)
}
