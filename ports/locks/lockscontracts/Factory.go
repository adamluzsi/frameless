package lockscontracts

import (
	"context"
	"github.com/adamluzsi/frameless/ports/locks"
	"github.com/adamluzsi/testcase"
	"testing"
	"time"
)

type Factory[Key comparable] func(tb testing.TB) FactorySubject[Key]

type FactorySubject[Key comparable] struct {
	Factory     locks.Factory[Key]
	MakeContext func() context.Context
	MakeKey     func() Key
}

func (c Factory[Key]) Spec(s *testcase.Spec) {
	subject := testcase.Let[FactorySubject[Key]](s, func(t *testcase.T) FactorySubject[Key] { return c(t) })

	s.Context("returned value behaves like a locks.Locker", Locker(func(tb testing.TB) LockerSubject {
		sub := c(tb)
		return LockerSubject{
			Locker:      sub.Factory.LockerFor(sub.MakeKey()),
			MakeContext: sub.MakeContext,
		}
	}).Spec)

	s.Test("result Lockers with different name don't interfere with each other", func(t *testcase.T) {
		var (
			ctx = subject.Get(t).MakeContext()
			l1  = subject.Get(t).Factory.LockerFor(subject.Get(t).MakeKey())
			l2  = subject.Get(t).Factory.LockerFor(subject.Get(t).MakeKey())
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
}

func (c Factory[Key]) Test(t *testing.T)      { c.Spec(testcase.NewSpec(t)) }
func (c Factory[Key]) Benchmark(b *testing.B) { c.Spec(testcase.NewSpec(b)) }
