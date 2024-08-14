package guardcontracts

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/spechelper"
	"go.llib.dev/testcase"
)

func LockerFactory[Key comparable](subject guard.LockerFactory[Key], opts ...LockerFactoryOption[Key]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[LockerFactoryConfig[Key]](opts)

	s.Test("returned value behaves like a locks.Locker", func(t *testcase.T) {
		testcase.RunSuite(t, Locker(subject.LockerFor(c.MakeKey(t))))
	})

	s.Test("result Lockers with different name don't interfere with each other", func(t *testcase.T) {
		var (
			ctx = c.MakeContext()
			l1  = subject.LockerFor(c.MakeKey(t))
			l2  = subject.LockerFor(c.MakeKey(t))
		)
		t.Must.Within(3*time.Second, func(context.Context) {
			lockCtx1, err := l1.Lock(ctx)
			t.Must.NoError(err)
			lockCtx2, err := l2.Lock(ctx)
			t.Must.NoError(err)
			t.Must.NoError(l2.Unlock(lockCtx1))
			t.Must.NoError(l1.Unlock(lockCtx2))
		})
	})

	return s.AsSuite("Factory")
}

type LockerFactoryOption[Key comparable] interface {
	option.Option[LockerFactoryConfig[Key]]
}

type LockerFactoryConfig[Key comparable] struct {
	MakeContext func() context.Context
	MakeKey     func(testing.TB) Key
}

func (c *LockerFactoryConfig[Key]) Init() {
	c.MakeContext = context.Background
	c.MakeKey = spechelper.MakeValue[Key]
}

func (c LockerFactoryConfig[Key]) Configure(t *LockerFactoryConfig[Key]) {
	option.Configure(c, t)
}
