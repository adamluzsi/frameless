package contextkit_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/testcase"
)

var _ context.Context = contextkit.Detach(context.Background())

var rnd = random.New(random.CryptoSeed{})

func TestDetached(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		parent = testcase.Let(s, func(t *testcase.T) context.Context {
			return context.Background()
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) context.Context {
		return contextkit.Detach(parent.Get(t))
	})

	s.Describe(".Deadline", func(s *testcase.Spec) {
		act := func(t *testcase.T) (deadline time.Time, ok bool) {
			return subject.Get(t).Deadline()
		}

		s.Then("no deadline returned", func(t *testcase.T) {
			deadline, ok := act(t)
			t.Must.False(ok)
			t.Must.Empty(deadline)
		})

		s.When("parent deadline is reached", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(-1*time.Second))
				t.Cleanup(cancelFunc)
				_, ok := ctx.Deadline()
				t.Must.True(ok)
				return ctx
			})

			s.Then("no deadline returned", func(t *testcase.T) {
				deadline, ok := act(t)
				t.Must.False(ok)
				t.Must.Empty(deadline)
			})
		})
	})

	s.Describe(".Done", func(s *testcase.Spec) {
		act := func(t *testcase.T) <-chan struct{} {
			return subject.Get(t).Done()
		}

		s.Then("it is not done", func(t *testcase.T) {
			select {
			case <-act(t):
				t.FailNow()
			default:
			}
		})

		s.When("parent context is done", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				<-ctx.Done()
				return ctx
			})

			s.Then("no deadline returned", func(t *testcase.T) {
				select {
				case <-act(t):
					t.FailNow()
				default:
				}
			})
		})
	})

	s.Describe(".Err", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Err()
		}

		s.Then("there is no error", func(t *testcase.T) {
			t.Must.NoError(act(t))
		})

		s.When("parent context has an error due to context cancellation", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				t.Must.NotNil(ctx.Err())
				return ctx
			})

			s.Then("there is no error", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})
		})

		s.When("parent context has an error due to deadline exceed", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithDeadline(parent.Super(t), time.Now())
				cancel()
				t.Must.ErrorIs(context.DeadlineExceeded, ctx.Err())
				return ctx
			})

			s.Then("there is no error", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})
		})
	})
}

func TestWithoutCancel(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		parent = testcase.Let(s, func(t *testcase.T) context.Context {
			return context.Background()
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) context.Context {
		return contextkit.WithoutCancel(parent.Get(t))
	})

	s.Describe(".Deadline", func(s *testcase.Spec) {
		act := func(t *testcase.T) (deadline time.Time, ok bool) {
			return subject.Get(t).Deadline()
		}

		s.Then("no deadline returned", func(t *testcase.T) {
			deadline, ok := act(t)
			t.Must.False(ok)
			t.Must.Empty(deadline)
		})

		s.When("parent deadline is reached", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancelFunc := context.WithDeadline(ctx, time.Now().Add(-1*time.Second))
				t.Cleanup(cancelFunc)
				_, ok := ctx.Deadline()
				t.Must.True(ok)
				return ctx
			})

			s.Then("no deadline returned", func(t *testcase.T) {
				deadline, ok := act(t)
				t.Must.False(ok)
				t.Must.Empty(deadline)
			})
		})
	})

	s.Describe(".Done", func(s *testcase.Spec) {
		act := func(t *testcase.T) <-chan struct{} {
			return subject.Get(t).Done()
		}

		s.Then("it is not done", func(t *testcase.T) {
			select {
			case <-act(t):
				t.FailNow()
			default:
			}
		})

		s.When("parent context is done", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				<-ctx.Done()
				return ctx
			})

			s.Then("no deadline returned", func(t *testcase.T) {
				select {
				case <-act(t):
					t.FailNow()
				default:
				}
			})
		})
	})

	s.Describe(".Err", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Err()
		}

		s.Then("there is no error", func(t *testcase.T) {
			t.Must.NoError(act(t))
		})

		s.When("parent context has an error due to context cancellation", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx := parent.Super(t)
				ctx, cancel := context.WithCancel(ctx)
				cancel()
				t.Must.NotNil(ctx.Err())
				return ctx
			})

			s.Then("there is no error", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})
		})

		s.When("parent context has an error due to deadline exceed", func(s *testcase.Spec) {
			parent.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithDeadline(parent.Super(t), time.Now())
				cancel()
				t.Must.ErrorIs(context.DeadlineExceeded, ctx.Err())
				return ctx
			})

			s.Then("there is no error", func(t *testcase.T) {
				t.Must.NoError(act(t))
			})
		})
	})
}

func ExampleValueHandler() {
	type MyContextKey struct{}
	type MyValueType string

	vic := contextkit.ValueHandler[MyContextKey, MyValueType]{}

	var ctx = context.Background() // empty context

	v, ok := vic.Lookup(ctx)
	_, _ = v, ok // "", false

	ctx = vic.ContextWith(ctx, "Hello, world!") // context with value

	v, ok = vic.Lookup(ctx)
	_, _ = v, ok // "Hello, world!", true
}

func TestValueHandler(t *testing.T) {
	type Key struct{}
	rnd := random.New(random.CryptoSeed{})
	t.Run("nil context", func(t *testing.T) {
		var ctx context.Context = nil
		vic := contextkit.ValueHandler[Key, string]{}
		v, ok := vic.Lookup(ctx)
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	t.Run("no value in context", func(t *testing.T) {
		var ctx context.Context = context.Background()
		vic := contextkit.ValueHandler[Key, string]{}
		v, ok := vic.Lookup(ctx)
		assert.False(t, ok)
		assert.Empty(t, v)
	})

	t.Run("value stored in context previously", func(t *testing.T) {
		var ctx = context.Background()
		vic := contextkit.ValueHandler[Key, string]{}
		exp := rnd.String()
		ctx = vic.ContextWith(ctx, exp)
		got, ok := vic.Lookup(ctx)
		assert.True(t, ok)
		assert.Equal(t, exp, got)
	})

	t.Run("valid nil value in the context", func(t *testing.T) {
		var ctx = context.Background()
		vic := contextkit.ValueHandler[Key, *string]{}
		var exp *string
		ctx = vic.ContextWith(ctx, exp)
		got, ok := vic.Lookup(ctx)
		assert.True(t, ok)
		assert.Equal(t, exp, got)
	})
}

func ExampleMerge() {
	type key string

	var (
		ctx1          = context.WithValue(context.Background(), key("foo"), 42)
		ctx2          = context.WithValue(context.Background(), key("bar"), 128)
		ctx3, cancel3 = context.WithTimeout(context.Background(), time.Hour)
	)
	defer cancel3()

	ctx, cancel := contextkit.Merge(ctx1, ctx2, ctx3)
	defer cancel()
	_ = ctx.Value(key("foo")) // 42
	_ = ctx.Value(key("bar")) // 128
	_, _ = ctx.Deadline()     // Deadline = deadline time value; OK=true
}

func TestMerge(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		type key string

		var (
			ctx1          = context.WithValue(context.Background(), key("foo"), 42)
			ctx2          = context.WithValue(context.Background(), key("bar"), 128)
			ctx3, cancel3 = context.WithTimeout(context.Background(), time.Hour)
		)
		defer cancel3()

		ctx, cancel := contextkit.Merge(ctx1, ctx2, ctx3)
		defer cancel()

		assert.Equal[any](t, ctx.Value(key("foo")), 42)
		assert.Equal[any](t, ctx.Value(key("bar")), 128)
		dl, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.NotEmpty(t, dl)
	})

	type Key string
	t.Run("two context - values merged", func(t *testing.T) {
		ctx, cancel := contextkit.Merge(
			context.WithValue(context.Background(), Key("foo"), 1),
			context.WithValue(context.Background(), Key("bar"), 2),
		)
		defer cancel()
		assert.Equal[any](t, ctx.Value(Key("foo")), 1)
		assert.Equal[any](t, ctx.Value(Key("bar")), 2)
		assert.NoError(t, ctx.Err())
		_, ok := ctx.Deadline()
		assert.False(t, ok)
		assert.Within(t, time.Second, func(actx context.Context) {
			select {
			case <-actx.Done():
			case <-ctx.Done():
			default: // OK
			}
		})
	})

	t.Run("order defines priority - value retrieval - last takes priority", func(t *testing.T) {
		ctx, cancel := contextkit.Merge(
			context.WithValue(context.Background(), Key("foo"), 42),
			context.WithValue(context.Background(), Key("foo"), 24),
		)
		defer cancel()
		assert.Equal[any](t, ctx.Value(Key("foo")), 24)
	})

	t.Run("2. is cancelled", func(t *testing.T) {
		ctx, cancel := contextkit.Merge(
			context.Background(),
			func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
		)
		defer cancel()
		assert.Within(t, time.Second, func(actx context.Context) {
			select {
			case <-actx.Done():
			case <-ctx.Done():
				// OK
			}
		})
		assert.Error(t, ctx.Err())
	})

	t.Run("1. is cancelled", func(t *testing.T) {
		ctx, cancel := contextkit.Merge(
			func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			context.Background(),
		)
		defer cancel()
		assert.Within(t, time.Second, func(actx context.Context) {
			select {
			case <-actx.Done():
			case <-ctx.Done():
				// OK
			}
		})
		assert.Error(t, ctx.Err())
	})

	t.Run("1. deadline", func(t *testing.T) {
		exp := time.Now().Add(time.Hour)
		ctx, cancel := contextkit.Merge(
			func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), exp)
				t.Cleanup(cancel)
				return ctx
			}(),
			context.Background(),
		)
		defer cancel()

		dl, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.NotEmpty(t, dl)
		assert.Equal(t, exp, dl)
	})

	t.Run("2. deadline", func(t *testing.T) {
		exp := time.Now().Add(time.Hour)
		ctx, cancel := contextkit.Merge(
			context.Background(),
			func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), exp)
				t.Cleanup(cancel)
				return ctx
			}(),
		)
		defer cancel()
		dl, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.NotEmpty(t, dl)
		assert.Equal(t, exp, dl)
	})

	t.Run("2+", func(t *testing.T) {
		expDL := time.Now().Add(time.Hour)

		ctx, cancel := contextkit.Merge(
			context.Background(),
			context.WithValue(context.Background(), Key("foo"), 42),
			func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), expDL)
				t.Cleanup(cancel)
				return ctx
			}(),
		)

		defer cancel()
		assert.Equal[any](t, ctx.Value(Key("foo")), 42)
		dl, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.NotEmpty(t, dl)
		assert.Equal(t, expDL, dl)
	})

	t.Run("no context provided", func(t *testing.T) {
		ctx, cancel := contextkit.Merge()
		assert.NotNil(t, cancel)
		cancel()
		assert.NotNil(t, ctx)
		assert.NoError(t, ctx.Err())
		_, ok := ctx.Deadline()
		assert.False(t, ok)
		assert.Within(t, time.Second, func(actx context.Context) {
			select {
			case <-actx.Done():
			case <-ctx.Done():
			default: // OK
			}
		})
		assert.NotPanic(t, func() {
			cancel()
		})
	})

	t.Run("cancel will clean up the hanging Merge goroutine", func(t *testing.T) {
		assert.Eventually(t, 5, func(t testing.TB) {
			var initialNumGoroutine int = runtime.NumGoroutine()
			for i := 0; i < 1024; i++ {
				ngrc := runtime.NumGoroutine()
				if ngrc == initialNumGoroutine {
					break
				}
				initialNumGoroutine = ngrc
			}

			ctx, cancel := contextkit.Merge(context.Background(), context.Background(), context.Background())
			assert.NotNil(t, ctx)
			defer cancel()

			var afterMergeNumGoroutine int = runtime.NumGoroutine()
			for i := 0; i < 1024; i++ {
				ngrc := runtime.NumGoroutine()
				if ngrc == afterMergeNumGoroutine {
					break
				}
				afterMergeNumGoroutine = ngrc
			}

			cancel()

			// assert Eventually don't use go routines, should be safe to use
			assert.Eventually(t, time.Millisecond, func(it testing.TB) {
				currentNumGoroutine := runtime.NumGoroutine()
				assert.True(it,
					afterMergeNumGoroutine < currentNumGoroutine ||
						initialNumGoroutine < afterMergeNumGoroutine)
			})
		})
	})

	t.Run("single context", func(t *testing.T) {
		pctx, pcancel := context.WithCancel(context.Background())
		defer pcancel()

		ctx, cancel := contextkit.Merge(pctx)
		assert.NotNil(t, cancel)
		defer cancel()
		assert.NotNil(t, ctx)

		assert.NoError(t, ctx.Err())

		pcancel()

		assert.ErrorIs(t, ctx.Err(), pctx.Err())
	})

	t.Run("race", func(t *testing.T) {
		ctx, cancel := contextkit.Merge(context.Background(), context.Background(), context.Background())
		defer cancel()

		testcase.Race(func() {
			<-ctx.Done()
		}, func() {
			<-ctx.Done()
		}, func() {
			ctx.Err()
		}, func() {
			cancel()
		})
	})
}

func TestWithoutValues(t *testing.T) {
	type ctxKey struct{}

	t.Run("when value present in the base context it is not found", func(t *testing.T) {
		bctx := context.WithValue(context.Background(), ctxKey{}, "bar")
		assert.NotNil(t, bctx.Value(ctxKey{}))
		assert.Nil(t, contextkit.WithoutValues(bctx).Value(ctxKey{}))
	})

	t.Run("Error is propagated", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		assert.ErrorIs(t, ctx.Err(), contextkit.WithoutValues(ctx).Err())
	})

	t.Run("deadline is propagated", func(t *testing.T) {
		dl := time.Now().Add(time.Hour)
		ctx, cancel := context.WithDeadline(context.Background(), dl)
		defer cancel()
		got, ok := ctx.Deadline()
		assert.True(t, ok)
		assert.Equal(t, got, dl)
	})

	t.Run("done propagated", func(t *testing.T) {
		bctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		subject := contextkit.WithoutValues(bctx)

		assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
			select {
			case <-ctx.Done():
			case <-subject.Done(): // hangs since it is not done yet
			}
		})

		cancel()

		assert.Within(t, time.Millisecond, func(ctx context.Context) {
			select {
			case <-ctx.Done():
			case <-subject.Done(): // instantly returns since it is cancelled
			}
		})
	})
}
