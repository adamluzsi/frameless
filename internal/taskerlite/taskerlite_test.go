package taskerlite_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/internal/taskerlite"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

const blockCheckWaitTime = 42 * time.Millisecond

var _ taskerlite.Runnable = taskerlite.Concurrence[func()]()

func TestConcurrence_Run(t *testing.T) {
	type Key struct{ V string }

	var (
		rnd   = random.New(random.CryptoSeed{})
		key   = Key{V: rnd.String()}
		value = rnd.String()
		ctx   = context.WithValue(context.Background(), key, value)
	)

	t.Run("when sequence is uninitialized", func(t *testing.T) {
		var s = taskerlite.Concurrence[func()]()
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("when every task succeed", func(t *testing.T) {
		s := taskerlite.Concurrence(
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("the tasker are executed", func(t *testing.T) {
		var out int32
		s := taskerlite.Concurrence(
			func(ctx context.Context) error { atomic.AddInt32(&out, 1); return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 10); return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 100); return nil },
		)
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))

			assert.Eventually(t, time.Second, func(it testing.TB) {
				assert.Equal(it, int32(111), atomic.LoadInt32(&out))
			})
		})
	})

	t.Run("the .Run will wait until the tasker are done", func(t *testing.T) {
		s := taskerlite.Concurrence(
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		)
		assert.NotWithin(t, blockCheckWaitTime, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})

	t.Run("the tasker are executed concurrently", func(t *testing.T) {
		var (
			out   int32
			done1 = make(chan struct{})
			done2 = make(chan struct{})
			done3 = make(chan struct{})
		)
		s := taskerlite.Concurrence(
			func(ctx context.Context) error { atomic.AddInt32(&out, 1); <-done1; return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 10); <-done2; return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 100); <-done3; return nil },
		)

		go func() { assert.NoError(t, s.Run(ctx)) }()
		assert.Eventually(t, time.Second, func(it testing.TB) {
			assert.Equal(it, int32(111), atomic.LoadInt32(&out))
		})
	})

	t.Run("when a task fails, it will interrupt the rest of the concurrent task and we get back the failure", func(t *testing.T) {
		expErr := rnd.Error()
		s := taskerlite.Concurrence(
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { return expErr },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		)
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.ErrorIs(t, expErr, s.Run(ctx))
		})
	})

	t.Run("when multiple task fails, then all collected and merged into a single error", func(t *testing.T) {
		var (
			expErr1 = rnd.Error()
			expErr2 = rnd.Error()
			expErr3 = rnd.Error()
		)
		s := taskerlite.Concurrence(
			func(ctx context.Context) error { return expErr1 },
			func(ctx context.Context) error { return expErr2 },
			func(ctx context.Context) error { return expErr3 },
		)
		assert.Within(t, time.Second, func(ctx context.Context) {
			gotErr := s.Run(ctx)
			assert.ErrorIs(t, expErr1, gotErr)
			assert.ErrorIs(t, expErr2, gotErr)
			assert.ErrorIs(t, expErr3, gotErr)
		})
	})

	t.Run("when task fails with context cancellation, it is not reported back", func(t *testing.T) {
		s := taskerlite.Concurrence(
			func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
			func(ctx context.Context) error { return context.Canceled },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		)
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})

	t.Run("tasker will receive the input context", func(t *testing.T) {
		assertContext := func(ctx context.Context) {
			assert.NotNil(t, ctx)
			gotValue, ok := ctx.Value(key).(string)
			assert.True(t, ok, "key contains a value")
			assert.Equal(t, value, gotValue, "the received value is what we expect")
		}
		s := taskerlite.Concurrence(
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
		)
		assert.Within(t, time.Second, func(context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})
}

var _ taskerlite.Runnable = (taskerlite.Task)(nil)

func TestToTask_smoke(t *testing.T) {
	expErr := random.New(random.CryptoSeed{}).Error()
	task := taskerlite.ToTask(func() error { return expErr })
	assert.NotNil(t, task)
	assert.Equal(t, expErr, task(context.Background()))
}

func TestToTask(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})

	t.Run("on Task", func(t *testing.T) {
		assert.NotNil(t, taskerlite.ToTask(func(ctx context.Context) error { return nil }))
		expErr := rnd.Error()
		type key struct{}
		val := rnd.String()
		assert.Equal(t, expErr, taskerlite.ToTask(func(ctx context.Context) error { return expErr })(context.Background()))
		assert.NoError(t, taskerlite.ToTask(func(ctx context.Context) error {
			assert.Equal(t, val, ctx.Value(key{}).(string))
			return nil
		})(context.WithValue(context.Background(), key{}, val)))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, taskerlite.ToTask(func() error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, taskerlite.ToTask(func() error { return expErr })(context.Background()))
	})

	t.Run("on func(context.Context)", func(t *testing.T) {
		assert.NotNil(t, taskerlite.ToTask(func(ctx context.Context) {}))
		var ran bool
		assert.NoError(t, taskerlite.ToTask(func(ctx context.Context) { ran = true })(context.Background()))
		assert.True(t, ran)
		assert.NotWithin(t, blockCheckWaitTime, func(ctx context.Context) {
			assert.NoError(t, taskerlite.ToTask(func(ctx context.Context) { <-ctx.Done() })(ctx))
		})
		type key struct{}
		assert.NoError(t, taskerlite.ToTask(func(ctx context.Context) {
			assert.Equal(t, any(42), ctx.Value(key{}))
		})(context.WithValue(context.Background(), key{}, 42)))
	})

	t.Run("on func()", func(t *testing.T) {
		var ran bool
		assert.NoError(t, taskerlite.ToTask(func() { ran = true })(context.Background()))
		assert.True(t, ran)
	})

	t.Run("on *Runnable", func(t *testing.T) {
		expErr := rnd.Error()
		var r taskerlite.Runnable = taskerlite.Sequence(func(ctx context.Context) error { return expErr })
		task := taskerlite.ToTask(&r)
		assert.NotNil(t, task)
		assert.ErrorIs(t, expErr, task(context.Background()))
		type key struct{}
		r = taskerlite.Sequence(func(ctx context.Context) {
			assert.Equal(t, any(42), ctx.Value(key{}))
		})
		assert.NoError(t, taskerlite.ToTask(&r)(context.WithValue(context.Background(), key{}, 42)))
	})
}

func TestToTasks_smoke(t *testing.T) {
	expErr := random.New(random.CryptoSeed{}).Error()
	task := taskerlite.ToTask(func() error { return expErr })
	assert.NotNil(t, task)
	tasks := taskerlite.ToTasks([]taskerlite.Task{task})
	assert.NotNil(t, tasks)
	assert.True(t, len(tasks) == 1)
	assert.NotNil(t, tasks[0])
	assert.Equal(t, expErr, tasks[0](context.Background()))
}

var _ taskerlite.Runnable = taskerlite.Sequence[func()]()

func TestSequence_Run(t *testing.T) {
	type Key struct{ V string }

	var (
		rnd   = random.New(random.CryptoSeed{})
		key   = Key{V: rnd.String()}
		value = rnd.String()
		ctx   = context.WithValue(context.Background(), key, value)
	)

	t.Run("when sequence is uninitialized", func(t *testing.T) {
		var s = taskerlite.Sequence[func()]()
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("when every task succeed", func(t *testing.T) {
		s := taskerlite.Sequence(
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("the tasker in the Sequence are executed in a sequence order", func(t *testing.T) {
		var out []int
		s := taskerlite.Sequence(
			func(ctx context.Context) error { out = append(out, 1); return nil },
			func(ctx context.Context) error { out = append(out, 2); return nil },
			func(ctx context.Context) error { out = append(out, 3); return nil },
		)
		assert.NoError(t, s.Run(ctx))
		assert.Equal(t, []int{1, 2, 3}, out)
	})

	t.Run("when a task fails, it breaks the sequence and we get back the failure", func(t *testing.T) {
		expErr := rnd.Error()
		var out []int
		s := taskerlite.Sequence(
			func(ctx context.Context) error { out = append(out, 1); return nil },
			func(ctx context.Context) error { out = append(out, 2); return expErr },
			func(ctx context.Context) error { out = append(out, 3); return nil },
		)
		assert.ErrorIs(t, expErr, s.Run(ctx))
		assert.Equal(t, []int{1, 2}, out)
	})

	t.Run("tasker will receive the input context", func(t *testing.T) {
		assertContext := func(ctx context.Context) {
			assert.NotNil(t, ctx)
			gotValue, ok := ctx.Value(key).(string)
			assert.True(t, ok, "key contains a value")
			assert.Equal(t, value, gotValue, "the received value is what we expect")
		}
		s := taskerlite.Sequence(
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})
}
