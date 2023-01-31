package lockscontracts

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/ports/locks"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/let"
)

var Waiter = assert.Waiter{
	WaitDuration: time.Millisecond,
	Timeout:      5 * time.Second,
}

type Locker struct {
	MakeSubject func(testing.TB) locks.Locker
	MakeContext func(testing.TB) context.Context
}

func (c Locker) Spec(s *testcase.Spec) {
	const withinTimeout = time.Second

	subject := let.With[locks.Locker](s, c.MakeSubject)

	s.Describe(".Lock", func(s *testcase.Spec) {
		var (
			Context = let.With[context.Context](s, c.MakeContext)
		)
		act := func(t *testcase.T) (context.Context, error) {
			ctx, err := subject.Get(t).Lock(Context.Get(t))
			if err == nil {
				t.Defer(subject.Get(t).Unlock, ctx)
			}
			return ctx, err
		}

		s.Then("it locks successfully and returns a context that works with Unlock", func(t *testcase.T) {
			ctx, err := act(t)
			t.Must.NoError(err)
			t.Must.NotNil(ctx)
			t.Must.NoError(ctx.Err())
			t.Must.NoError(subject.Get(t).Unlock(ctx))
		})

		s.Then("calling lock will prevent other lock acquisitions", func(t *testcase.T) {
			ctx, err := act(t)
			t.Must.NoError(err)

			var isLocked int32
			go func() {
				ctx, err := subject.Get(t).Lock(context.Background())
				t.Must.NoError(err)
				t.Must.NotNil(ctx)
				t.Must.NoError(ctx.Err())
				t.Must.NoError(subject.Get(t).Unlock(ctx))
				atomic.AddInt32(&isLocked, 1)
			}()

			t.Random.Repeat(3, 7, Waiter.Wait)
			t.Must.Equal(int32(0), atomic.LoadInt32(&isLocked))

			t.Must.NoError(subject.Get(t).Unlock(ctx))
			t.Eventually(func(it assert.It) {
				// after unlock, the other Lock call unblocks
				it.Must.Equal(int32(1), atomic.LoadInt32(&isLocked))
			})
		})

		s.Then("calling unlock not with the locked context will yield an error", func(t *testcase.T) {
			lctx, err := act(t)
			t.Must.NoError(err)
			t.Must.ErrorIs(locks.ErrNoLock, subject.Get(t).Unlock(c.MakeContext(t)))
			t.Must.NoError(subject.Get(t).Unlock(lctx))
		})

		s.When("context is already done", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := Context.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				return ctx
			})

			s.Then("it will return back with the context error", func(t *testcase.T) {
				_, err := act(t)
				t.Must.ErrorIs(Context.Get(t).Err(), err)
			})
		})

		s.When("the current context already a lock context", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := Context.Super(t)
				lockCtx, err := subject.Get(t).Lock(ctx)
				t.Must.NoError(err)
				t.Defer(subject.Get(t).Unlock, lockCtx)
				return lockCtx
			})

			s.Then("since we have it already the lock ownership, it returns without doing much", func(t *testcase.T) {
				t.Must.Within(withinTimeout, func(ctx context.Context) {
					ctx, err := act(t)
					t.Must.NoError(err)
					t.Must.NotNil(ctx)
					t.Must.NoError(subject.Get(t).Unlock(ctx))
				})
			})
		})
	})

	s.Describe(".Unlock", func(s *testcase.Spec) {
		var (
			Context = testcase.Let[context.Context](s, nil)
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Unlock(Context.Get(t))
		}

		s.When("context is a lock context, made by a .Lock call", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := c.MakeContext(t)
				ctx, err := subject.Get(t).Lock(ctx)
				t.Must.NoError(err)
				t.Defer(subject.Get(t).Unlock, ctx)
				return ctx
			})

			s.Then("it unlocks without an issue", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})

			s.Then("unlocks multiple time without an issue", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					t.Must.NoError(act(t))
				})
			})

			s.Then("after unlock, the context is cancelled", func(t *testcase.T) {
				t.Must.NoError(act(t))
				t.Must.Error(Context.Get(t).Err())
			})

			s.And("context is cancelled during locking", func(s *testcase.Spec) {
				Context.Let(s, func(t *testcase.T) context.Context {
					ctx, cancel := context.WithCancel(c.MakeContext(t))
					ctx, err := subject.Get(t).Lock(ctx)
					t.Must.NoError(err)
					t.Defer(subject.Get(t).Unlock, ctx)
					cancel()
					return ctx
				})

				s.Then("it will return back with the context error", func(t *testcase.T) {
					t.Must.ErrorIs(Context.Get(t).Err(), act(t))
				})
			})
		})

		s.When("context is not issued by a .Lock call", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				return c.MakeContext(t)
			})

			s.Then("it raise ErrNoLock error", func(t *testcase.T) {
				t.Must.ErrorIs(locks.ErrNoLock, act(t))
			})
		})
	})
}

func (c Locker) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Locker) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}
