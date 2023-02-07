package lockscontracts

import (
	"context"
	"github.com/adamluzsi/frameless/ports/locks"
	"github.com/adamluzsi/testcase"
	"testing"
	"time"
)

type Factory[Key comparable] struct {
	MakeSubject func(tb testing.TB) locks.Factory[Key]
	MakeContext func(tb testing.TB) context.Context
	MakeKey     func(tb testing.TB) Key
}

func (c Factory[Key]) Spec(s *testcase.Spec) {
	s.Context("returned value behaves like a locks.Locker", Locker{
		MakeSubject: func(tb testing.TB) locks.Locker {
			t := tb.(*testcase.T)
			lockerKey := testcase.Var[Key]{
				ID: "lockers.LockerFor's name value",
				Init: func(t *testcase.T) Key {
					return c.MakeKey(t)
				},
			}
			return c.MakeSubject(tb).LockerFor(lockerKey.Get(t))
		},
		MakeContext: c.MakeContext,
	}.Spec)

	s.Test("result Lockers with different name don't interfere with each other", func(t *testcase.T) {
		var (
			ctx = c.MakeContext(t)
			l1  = c.MakeSubject(t).LockerFor(c.MakeKey(t))
			l2  = c.MakeSubject(t).LockerFor(c.MakeKey(t))
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
