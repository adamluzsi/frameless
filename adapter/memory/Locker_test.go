package memory_test

import (
	"context"
	"testing"

	"go.llib.dev/testcase"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/guard/guardcontracts"
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
	guardcontracts.Locker(memory.NewLocker()).Test(t)
}

func TestLockerFactory(t *testing.T) {
	guardcontracts.LockerFactory[string](memory.NewLockerFactory[string]()).Test(t)
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
