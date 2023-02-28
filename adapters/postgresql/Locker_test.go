package postgresql_test

import (
	"context"
	"database/sql"
	"github.com/adamluzsi/frameless/ports/migration"
	"log"
	"os"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"
	"github.com/adamluzsi/frameless/ports/locks"
	lockscontracts "github.com/adamluzsi/frameless/ports/locks/lockscontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

func ExampleLocker() {
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	l := postgresql.Locker{
		Name: "my-lock",
		DB:   db,
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
	db, err := sql.Open("postgres", spechelper.DatabaseDSN(t))
	assert.NoError(t, err)

	lockscontracts.Locker{
		MakeSubject: func(tb testing.TB) locks.Locker {
			t := testcase.ToT(&tb)
			l := postgresql.Locker{
				Name: t.Random.StringNC(5, random.CharsetAlpha()),
				DB:   db,
			}
			assert.NoError(tb, l.Migrate(context.Background()))
			return l
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Test(t)
}

func ExampleLockerFactory() {
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	lockerFactory := postgresql.LockerFactory[string]{DB: db}
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
	db := OpenDB(t)

	lockscontracts.Factory[string]{
		MakeSubject: func(tb testing.TB) locks.Factory[string] {
			lockerFactory := postgresql.LockerFactory[string]{DB: db}
			assert.NoError(tb, lockerFactory.Migrate(context.Background()))
			return lockerFactory
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeKey: func(tb testing.TB) string {
			return tb.(*testcase.T).Random.String()
		},
	}.Test(t)

	lockscontracts.Factory[int]{
		MakeSubject: func(tb testing.TB) locks.Factory[int] {
			lockerFactory := postgresql.LockerFactory[int]{DB: db}
			assert.NoError(tb, lockerFactory.Migrate(context.Background()))
			return lockerFactory
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeKey: func(tb testing.TB) int {
			return tb.(*testcase.T).Random.Int()
		},
	}.Test(t)
}
