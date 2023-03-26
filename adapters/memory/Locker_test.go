package memory_test

import (
	"context"
	"github.com/adamluzsi/testcase"
	"testing"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/locks/lockscontracts"
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
	lockscontracts.Locker(func(tb testing.TB) lockscontracts.LockerSubject {
		return lockscontracts.LockerSubject{
			Locker:      memory.NewLocker(),
			MakeContext: context.Background,
		}
	}).Test(t)
}

func TestLockerFactory(t *testing.T) {
	lockscontracts.Factory[string](func(tb testing.TB) lockscontracts.FactorySubject[string] {
		return lockscontracts.FactorySubject[string]{
			Factory:     memory.NewLockerFactory[string](),
			MakeContext: context.Background,
			MakeKey:     tb.(*testcase.T).Random.String,
		}
	}).Test(t)
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
