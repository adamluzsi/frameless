package memory_test

import (
	"context"
	"github.com/adamluzsi/testcase"
	"testing"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/locks"
	lockscontracts "github.com/adamluzsi/frameless/ports/locks/lockscontracts"
)

func ExampleLocker() {
	l := memory.NewLocker()

	ctx, err := l.Lock(context.Background())
	if err != nil {
		panic(err)
	}

	if err := l.Unlock(ctx); err != nil {
		panic(err)
	}
}

func TestLocker(t *testing.T) {
	lockscontracts.Locker{
		MakeSubject: func(tb testing.TB) locks.Locker {
			return memory.NewLocker()
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Test(t)
}

func TestLockerFactory(t *testing.T) {
	lockscontracts.Factory[string]{
		MakeSubject: func(tb testing.TB) locks.Factory[string] {
			return memory.NewLockerFactory[string]()
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeKey: func(tb testing.TB) string {
			return tb.(*testcase.T).Random.String()
		},
	}.Test(t)
}

func TestNewLockerFactory_race(tt *testing.T) {
	t := testcase.NewT(tt, nil)
	lf := memory.NewLockerFactory[string]()

	const constKey = "const"
	testcase.Race(func() {
		lf.LockerFor(t.Random.String())
	}, func() {
		lf.LockerFor(t.Random.String())
	}, func() {
		lf.LockerFor(constKey)
	}, func() {
		lf.LockerFor(constKey)
	})
}
