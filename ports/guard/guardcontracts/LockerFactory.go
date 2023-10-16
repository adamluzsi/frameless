package guardcontracts

import (
	"context"
	"go.llib.dev/frameless/internal/suites"
	"go.llib.dev/frameless/ports/guard"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/let"
	"testing"
	"time"
)

type LockerFactorySubject[Key comparable] struct {
	LockerFactory guard.LockerFactory[Key]
	MakeContext   func() context.Context
	MakeKey       func() Key
}

func LockerFactory[Key comparable](mk func(tb testing.TB) LockerFactorySubject[Key]) suites.Suite {
	s := testcase.NewSpec(nil, testcase.AsSuite("Factory"))

	subject := let.With[LockerFactorySubject[Key]](s, mk)
	
	s.Context("returned value behaves like a locks.Locker", Locker(func(tb testing.TB) LockerSubject {
		sub := mk(tb)
		return LockerSubject{
			Locker:      sub.LockerFactory.LockerFor(sub.MakeKey()),
			MakeContext: sub.MakeContext,
		}
	}).Spec)

	s.Test("result Lockers with different name don't interfere with each other", func(t *testcase.T) {
		var (
			ctx = subject.Get(t).MakeContext()
			l1  = subject.Get(t).LockerFactory.LockerFor(subject.Get(t).MakeKey())
			l2  = subject.Get(t).LockerFactory.LockerFor(subject.Get(t).MakeKey())
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

	return s.AsSuite()
}
