package guardcontract

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

// TODO:
// - add coverage for context cancellation during lock hanging

var Timeout = testcase.Var[time.Duration]{
	ID: "assertion Timeout.Get(t)",
	Init: func(t *testcase.T) time.Duration {
		return time.Second
	},
}

func Locker(subject guard.Locker, opts ...LockerOption) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[LockerConfig](opts)

	s.After(func(t *testcase.T) {
		_ = subject.Unlock(c.MakeContext(t))
	})

	s.Describe(".Lock", func(s *testcase.Spec) {
		var (
			Context = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
				return c.MakeContext(t)
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
			var lockContext context.Context
			assert.Within(t, Timeout.Get(t), func(context.Context) {
				var err error
				lockContext, err = act(t)
				assert.NoError(t, err)
			})

			w := assert.NotWithin(t, Timeout.Get(t), func(context.Context) {
				ctx, err := subject.Lock(c.MakeContext(t))
				t.Must.NoError(err)
				t.Must.NotNil(ctx)
				t.Must.NoError(ctx.Err())
				t.Must.NoError(subject.Unlock(ctx))
			})

			t.Must.NoError(subject.Unlock(lockContext))

			// after unlock, the other Lock call unblocks
			assert.Within(t, Timeout.Get(t), func(context.Context) { w.Wait() })
		})

		s.Then("calling unlock not with the locked context will yield an error", func(t *testcase.T) {
			t.Must.ErrorIs(guard.ErrNoLock, subject.Unlock(c.MakeContext(t)))
		})

		s.When("the lock is already taken", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Within(t, Timeout.Get(t), func(context.Context) {
					lctx, err := act(t)
					assert.NoError(t, err)
					t.Defer(subject.Unlock, lctx)
				})
			})

			s.Then("the Locking will hangs", func(t *testcase.T) {
				d := t.Random.DurationBetween(250*time.Millisecond, time.Second)

				assert.NotWithin(t, d, func(ctx context.Context) {
					ctx, cancel := contextkit.Merge(ctx, Context.Get(t))
					defer cancel()
					subject.Lock(ctx)
				})
			})
		})

		s.When("context is already done", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := Context.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				return ctx
			})

			s.Then("it will return back with the context error", func(t *testcase.T) {
				assert.Within(t, Timeout.Get(t), func(ctx context.Context) {
					_, err := act(t)
					t.Must.ErrorIs(Context.Get(t).Err(), err)
				})
			})
		})

		s.When("the current context already a lock context", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				var lockContext context.Context
				assert.Within(t, Timeout.Get(t), func(ctx context.Context) {
					lockCtx, err := subject.Lock(Context.Super(t))
					t.Must.NoError(err)
					t.Defer(subject.Unlock, lockCtx)
					lockContext = lockCtx
				})
				return lockContext
			})

			s.Then("since we have it already the lock ownership, it returns without doing much", func(t *testcase.T) {
				t.Must.Within(Timeout.Get(t), func(context.Context) {
					ctx, err := act(t)
					t.Must.NoError(err)
					t.Must.NotNil(ctx)
					t.Must.NoError(subject.Unlock(ctx))
				})
			})
		})

		s.When("context has a value", func(s *testcase.Spec) {
			type ctxKey struct{}
			ctxVal := let.String(s)

			Context.Let(s, func(t *testcase.T) context.Context {
				c := Context.Super(t)
				return context.WithValue(c, ctxKey{}, ctxVal.Get(t))
			})

			s.Then("the lock context will contain the previously injected values", func(t *testcase.T) {
				ctx, err := act(t)
				t.Must.NoError(err)
				t.Must.NotNil(ctx)
				t.Must.NoError(ctx.Err())
				defer subject.Unlock(ctx)

				gotVal, ok := ctx.Value(ctxKey{}).(string)
				assert.True(t, ok, "expected that the stored value under the given key will be there")
				assert.Equal(t, ctxVal.Get(t), gotVal)
			})
		})
	})

	s.Describe(".Unlock", Unlocker(subject, subject.Lock).Spec)

	return s.AsSuite("Locker")
}

func NonBlockingLocker(subject guard.NonBlockingLocker, opts ...LockerOption) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[LockerConfig](opts)

	// Reuse tests from Locker contract for Unlock method
	s.Describe(".Unlock", Unlocker(subject, func(ctx context.Context) (context.Context, error) {
		for {
			lctx, ok, err := subject.TryLock(ctx)
			if err != nil {
				return nil, err
			}
			if ok {
				return lctx, nil
			}
		}
	}).Spec)

	s.Describe(".TryLock", func(s *testcase.Spec) {
		var (
			Context = testcase.Let[context.Context](s, func(t *testcase.T) context.Context {
				return c.MakeContext(t)
			})
		)

		act := func(t *testcase.T) (context.Context, bool, error) {
			var (
				ctx      context.Context
				acquired bool
				err      error
			)
			assert.Within(t, Timeout.Get(t), func(context.Context) {
				ctx, acquired, err = subject.TryLock(Context.Get(t))
				if acquired {
					t.Defer(subject.Unlock, ctx)
				}
			}, "TryLock was not suppose to hang in any possible scenario")
			return ctx, acquired, err
		}

		s.Then("it can acquire the lock", func(t *testcase.T) {
			ctx, isAcquired, err := act(t)
			t.Must.NoError(err)
			t.Must.True(isAcquired)
			t.Must.NotNil(ctx)
			t.Must.NoError(ctx.Err())
			t.Must.NoError(subject.Unlock(ctx))
		})

		s.Then("it returns true and a valid context if the lock is available", func(t *testcase.T) {
			ctx, acquired, err := act(t)
			t.Must.NoError(err)
			t.Must.True(acquired)
			t.Must.NotNil(ctx)
			t.Must.NoError(ctx.Err())
			t.Must.NoError(subject.Unlock(ctx))
		})

		s.When("the lock is already acquired", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				ctx, isAcquired, err := act(t)
				t.Must.NoError(err)
				t.Must.True(isAcquired)
				t.Must.NotNil(ctx)
				t.Must.NoError(ctx.Err())
				t.Defer(subject.Unlock, ctx)
			})

			s.Then("it returns immediately with false as the lock is not available", func(t *testcase.T) {
				ctx, acquired, err := act(t)
				t.Must.NoError(err)
				t.Must.False(acquired)
				t.Must.Nil(ctx)
			})
		})

		s.When("context is already done", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := Context.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				return ctx
			})

			s.Then("it returns back with the context error", func(t *testcase.T) {
				_, _, err := act(t)
				t.Must.ErrorIs(Context.Get(t).Err(), err)
			})
		})

		s.When("context has a value", func(s *testcase.Spec) {
			type ctxKey struct{}
			ctxVal := let.String(s)

			Context.Let(s, func(t *testcase.T) context.Context {
				c := Context.Super(t)
				return context.WithValue(c, ctxKey{}, ctxVal.Get(t))
			})

			s.Then("the acquired lock context will contain the previously injected values", func(t *testcase.T) {
				ctx, isAcquired, err := act(t)
				t.Must.NoError(err)
				t.Must.True(isAcquired)
				t.Must.NotNil(ctx)
				t.Must.NoError(ctx.Err())
				defer subject.Unlock(ctx)

				gotVal, ok := ctx.Value(ctxKey{}).(string)
				assert.True(t, ok, "expected that the stored value under the given key will be there")
				assert.Equal(t, ctxVal.Get(t), gotVal)
			})
		})
	})

	return s.AsSuite("NonBlockingLocker")
}

func Unlocker(subject guard.Unlocker, lock func(context.Context) (context.Context, error), opts ...LockerOption) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	var (
		Context = testcase.Let[context.Context](s, nil)
	)
	act := func(t *testcase.T) error {
		return subject.Unlock(Context.Get(t))
	}

	s.When("context is a lock context, made by a lock call", func(s *testcase.Spec) {
		Context.Let(s, func(t *testcase.T) context.Context {
			ctx := c.MakeContext(t)
			assert.Within(t, Timeout.Get(t), func(context.Context) {
				lctx, err := lock(ctx)
				t.Must.NoError(err)
				t.Defer(subject.Unlock, lctx)
				ctx = lctx
			}, "unable to lock, could it be that due to implementation issue, the previous test the lock in a locked state?")
			return ctx
		})

		s.Then("it unlocks without an issue", func(t *testcase.T) {
			t.Must.NoError(act(t))
		})

		s.Then("the already locked context should not hang locking it again", func(t *testcase.T) {
			assert.Within(t, Timeout.Get(t), func(context.Context) {
				ctx, err := lock(Context.Get(t))
				t.Must.NoError(err)
				t.Defer(subject.Unlock, ctx)
			})
		})

		s.Then("unlocks multiple time without an issue", func(t *testcase.T) {
			t.Random.Repeat(3, 7, func() {
				assert.Within(t, Timeout.Get(t), func(context.Context) {
					t.Must.NoError(act(t))
				})
			})
		})

		s.Then("after unlock, the context is cancelled", func(t *testcase.T) {
			t.Must.NoError(act(t))
			t.Must.Error(Context.Get(t).Err())
		})

		s.And("context is cancelled during locking", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(c.MakeContext(t))
				ctx, err := lock(ctx)
				t.Must.NoError(err)
				t.Defer(subject.Unlock, ctx)
				cancel()
				return ctx
			})

			s.Then("it will return back with the context error while also unlocking itself", func(t *testcase.T) {
				t.Must.ErrorIs(Context.Get(t).Err(), act(t))
				t.Must.Within(3*time.Second, func(ctx context.Context) {
					lock(ctx)
				})
			})
		})
	})

	s.When("context is not issued by a .Lock call", func(s *testcase.Spec) {
		Context.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})

		s.Then("it raise ErrNoLock error", func(t *testcase.T) {
			t.Must.ErrorIs(guard.ErrNoLock, act(t))
		})
	})

	return s.AsSuite("Unlocker")
}

type LockerOption interface {
	option.Option[LockerConfig]
}

type LockerConfig struct {
	MakeContext func(testing.TB) context.Context
	Waiter      assert.Waiter
}

func (c *LockerConfig) Init() {
	c.MakeContext = func(testing.TB) context.Context {
		return context.Background()
	}
	c.Waiter = assert.Waiter{
		WaitDuration: time.Millisecond,
		Timeout:      5 * time.Second,
	}
}

func (c LockerConfig) Configure(t *LockerConfig) {
	*t = reflectkit.MergeStruct(*t, c)
}

func LockerFactory[Key comparable](subject guard.LockerFactory[Key], opts ...LockerFactoryOption[Key]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Test("returned value behaves like a locks.Locker", func(t *testcase.T) {
		testcase.RunSuite(t, Locker(subject.LockerFor(c.MakeKey(t))))
	})

	s.Test("result Lock with different name don't interfere with each other", func(t *testcase.T) {
		var (
			ctx = c.MakeContext(t)
			k1  = c.MakeKey(t)
			k2  = random.Unique(func() Key { return c.MakeKey(t) }, k1)
			l1  = subject.LockerFor(k1)
			l2  = subject.LockerFor(k2)
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

	return s.AsSuite("LockerFactory")
}

func NonBlockingLockerFactory[Key comparable](subject guard.NonBlockingLockerFactory[Key], opts ...LockerFactoryOption[Key]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig(opts)

	s.Test("returned value behaves like a locks.NonBlockingLock", func(t *testcase.T) {
		testcase.RunSuite(t, NonBlockingLocker(subject.NonBlockingLockerFor(c.MakeKey(t)), c.LockerConfig))
	})

	s.Test("result NonBlockingLock with different name don't interfere with each other", func(t *testcase.T) {
		var (
			ctx = c.MakeContext(t)
			l1  = subject.NonBlockingLockerFor(c.MakeKey(t))
			l2  = subject.NonBlockingLockerFor(c.MakeKey(t))
		)
		t.Must.Within(3*time.Second, func(context.Context) {
			lockCtx1, ok, err := l1.TryLock(ctx)
			assert.NoError(t, err)
			assert.True(t, ok)
			t.Defer(l1.Unlock, lockCtx1)

			lockCtx2, ok, err := l2.TryLock(ctx)
			assert.NoError(t, err)
			assert.True(t, ok)
			t.Defer(l2.Unlock, lockCtx2)
		})
	})

	return s.AsSuite("NonBlockingLockerFactory")
}

type LockerFactoryOption[Key comparable] interface {
	option.Option[LockerFactoryConfig[Key]]
}

type LockerFactoryConfig[Key comparable] struct {
	MakeKey func(testing.TB) Key
	LockerConfig
}

func (c *LockerFactoryConfig[Key]) Init() {
	c.MakeContext = func(t testing.TB) context.Context {
		return context.Background()
	}
	c.MakeKey = spechelper.MakeValue[Key]
}

func (c LockerFactoryConfig[Key]) Configure(t *LockerFactoryConfig[Key]) {
	*t = reflectkit.MergeStruct(*t, c)
}
