package guardcontracts

import (
	"context"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func Locker(subject guard.Locker, opts ...LockerOption) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[LockerConfig](opts)

	const withinTimeout = time.Second

	s.After(func(t *testcase.T) {
		_ = subject.Unlock(c.MakeContext())
	})

	s.Describe(".Lock", func(s *testcase.Spec) {
		var (
			Context = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
				return c.MakeContext()
			})
		)
		act := func(t *testcase.T) (context.Context, error) {
			ctx, err := subject.Lock(Context.Get(t))
			if err == nil {
				t.Defer(subject.Unlock, ctx)
			}
			return ctx, err
		}

		s.Then("it locks successfully and returns a context that works with Unlock", func(t *testcase.T) {
			ctx, err := act(t)
			t.Must.NoError(err)
			t.Must.NotNil(ctx)
			t.Must.NoError(ctx.Err())
			t.Must.NoError(subject.Unlock(ctx))
		})

		s.Then("calling lock will prevent other lock acquisitions", func(t *testcase.T) {
			ctx, err := act(t)
			t.Must.NoError(err)

			var isLocked int32
			go func() {
				ctx, err := subject.Lock(context.Background())
				t.Must.NoError(err)
				t.Must.NotNil(ctx)
				t.Must.NoError(ctx.Err())
				t.Must.NoError(subject.Unlock(ctx))
				atomic.AddInt32(&isLocked, 1)
			}()

			t.Random.Repeat(3, 7, c.Waiter.Wait)
			t.Must.Equal(int32(0), atomic.LoadInt32(&isLocked))

			t.Must.NoError(subject.Unlock(ctx))
			t.Eventually(func(it *testcase.T) {
				// after unlock, the other Lock call unblocks
				it.Must.Equal(int32(1), atomic.LoadInt32(&isLocked))
			})
		})

		s.Then("calling unlock not with the locked context will yield an error", func(t *testcase.T) {
			lockContext, err := act(t)
			t.Must.NoError(err)
			t.Must.ErrorIs(guard.ErrNoLock, subject.Unlock(c.MakeContext()))
			t.Must.NoError(subject.Unlock(lockContext))
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
				lockCtx, err := subject.Lock(ctx)
				t.Must.NoError(err)
				t.Defer(subject.Unlock, lockCtx)
				return lockCtx
			})

			s.Then("since we have it already the lock ownership, it returns without doing much", func(t *testcase.T) {
				t.Must.Within(withinTimeout, func(context.Context) {
					ctx, err := act(t)
					t.Must.NoError(err)
					t.Must.NotNil(ctx)
					t.Must.NoError(subject.Unlock(ctx))
				})
			})
		})
	})

	s.Describe(".Unlock", func(s *testcase.Spec) {
		var (
			Context = testcase.Let[context.Context](s, nil)
		)
		act := func(t *testcase.T) error {
			return subject.Unlock(Context.Get(t))
		}

		s.When("context is a lock context, made by a .Lock call", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := c.MakeContext()
				assert.Within(t, 5*time.Second, func(context.Context) {
					lctx, err := subject.Lock(ctx)
					t.Must.NoError(err)
					ctx = lctx
				}, "unable to lock, could it be that due to implementation issue, the previous test the lock in a locked state?")
				ctx, err := subject.Lock(ctx)
				t.Must.NoError(err)
				t.Defer(subject.Unlock, ctx)
				return ctx
			})

			s.Then("it unlocks without an issue", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})

			s.Then("unlock can be called multiple times to make it convinent to use", func(t *testcase.T) {
				t.Random.Repeat(2, 8, func() {
					t.Must.NoError(act(t))
				})
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
					ctx, cancel := context.WithCancel(c.MakeContext())
					ctx, err := subject.Lock(ctx)
					t.Must.NoError(err)
					t.Defer(subject.Unlock, ctx)
					cancel()
					return ctx
				})

				s.Then("it will return back with the context error while also unlocking itself", func(t *testcase.T) {
					t.Must.ErrorIs(Context.Get(t).Err(), act(t))
					t.Must.Within(3*time.Second, func(ctx context.Context) {
						subject.Lock(ctx)
					})
				})
			})
		})

		s.When("context is not issued by a .Lock call", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				return c.MakeContext()
			})

			s.Then("it raise ErrNoLock error", func(t *testcase.T) {
				t.Must.ErrorIs(guard.ErrNoLock, act(t))
			})
		})
	})

	return s.AsSuite("Locker")
}

type LockerOption interface {
	option.Option[LockerConfig]
}

type LockerConfig struct {
	MakeContext func() context.Context
	Waiter      assert.Waiter
}

func (c *LockerConfig) Init() {
	c.MakeContext = context.Background
	c.Waiter = assert.Waiter{
		WaitDuration: time.Millisecond,
		Timeout:      5 * time.Second,
	}
}

func (c LockerConfig) Configure(t *LockerConfig) {
	option.Configure(c, t)
}
