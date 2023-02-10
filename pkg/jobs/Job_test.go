package jobs_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/jobs"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"sync/atomic"
	"testing"
	"time"
)

func TestToJob_smoke(t *testing.T) {
	expErr := random.New(random.CryptoSeed{}).Error()
	job := jobs.ToJob(func() error { return expErr })
	assert.NotNil(t, job)
	assert.Equal(t, expErr, job(context.Background()))
}

func TestToJob(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run("on Job", func(t *testing.T) {
		assert.NotNil(t, jobs.ToJob(jobs.Job(func(ctx context.Context) error { return nil })))
		expErr := rnd.Error()
		assert.Equal(t, expErr, jobs.ToJob(jobs.Job(func(ctx context.Context) error { return expErr }))(context.Background()))
		assert.NoError(t, jobs.ToJob(jobs.Job(func(ctx context.Context) error {
			assert.Equal(t, "v", ctx.Value("k").(string))
			return nil
		}))(context.WithValue(context.Background(), "k", "v")))
	})

	t.Run("on Job func", func(t *testing.T) {
		assert.NotNil(t, jobs.ToJob(func(ctx context.Context) error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, jobs.ToJob(func(ctx context.Context) error { return expErr })(context.Background()))
		assert.NoError(t, jobs.ToJob(func(ctx context.Context) error {
			assert.Equal(t, "v", ctx.Value("k").(string))
			return nil
		})(context.WithValue(context.Background(), "k", "v")))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, jobs.ToJob(func() error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, jobs.ToJob(func() error { return expErr })(context.Background()))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, jobs.ToJob(func() {}))
		assert.NoError(t, jobs.ToJob(func() {})(context.Background()))
	})

	t.Run("on Sequence", func(t *testing.T) {
		expErr := rnd.Error()
		job := jobs.ToJob(jobs.Sequence{func(ctx context.Context) error { return expErr }})
		assert.NotNil(t, job)
		assert.ErrorIs(t, expErr, job(context.Background()))
	})

	t.Run("on Concurrence", func(t *testing.T) {
		expErr := rnd.Error()
		job := jobs.ToJob(jobs.Concurrence{func(ctx context.Context) error { return expErr }})
		assert.NotNil(t, job)
		assert.ErrorIs(t, expErr, job(context.Background()))
	})

	t.Run(" on *Runnable", func(t *testing.T) {
		expErr := rnd.Error()
		var r jobs.Runnable = jobs.Sequence{func(ctx context.Context) error { return expErr }}
		job := jobs.ToJob(&r)
		assert.NotNil(t, job)
		assert.ErrorIs(t, expErr, job(context.Background()))
	})
}

func ExampleSequence() {
	err := jobs.Sequence{
		func(ctx context.Context) error {
			// first job to execute
			return nil
		},
		func(ctx context.Context) error {
			// follow-up job to execute
			return nil
		},
	}.Run(context.Background())
	_ = err
}

var _ jobs.Runnable = jobs.Sequence{}

func TestSequence_Run(t *testing.T) {
	var (
		rnd   = random.New(random.CryptoSeed{})
		key   = rnd.String()
		value = rnd.String()
		ctx   = context.WithValue(context.Background(), key, value)
	)

	t.Run("when sequence is uninitialized", func(t *testing.T) {
		var s jobs.Sequence
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("when every job succeed", func(t *testing.T) {
		s := jobs.Sequence{
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
		}
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("the jobs in the Sequence are executed in a sequence order", func(t *testing.T) {
		var out []int
		s := jobs.Sequence{
			func(ctx context.Context) error { out = append(out, 1); return nil },
			func(ctx context.Context) error { out = append(out, 2); return nil },
			func(ctx context.Context) error { out = append(out, 3); return nil },
		}
		assert.NoError(t, s.Run(ctx))
		assert.Equal(t, []int{1, 2, 3}, out)
	})

	t.Run("when a job fails, it breaks the sequence and we get back the failure", func(t *testing.T) {
		expErr := rnd.Error()
		var out []int
		s := jobs.Sequence{
			func(ctx context.Context) error { out = append(out, 1); return nil },
			func(ctx context.Context) error { out = append(out, 2); return expErr },
			func(ctx context.Context) error { out = append(out, 3); return nil },
		}
		assert.ErrorIs(t, expErr, s.Run(ctx))
		assert.Equal(t, []int{1, 2}, out)
	})

	t.Run("jobs will receive the input context", func(t *testing.T) {
		assertContext := func(ctx context.Context) {
			assert.NotNil(t, ctx)
			gotValue, ok := ctx.Value(key).(string)
			assert.True(t, ok, "key contains a value")
			assert.Equal(t, value, gotValue, "the received value is what we expect")
		}
		s := jobs.Sequence{
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
		}
		assert.NoError(t, s.Run(ctx))
	})
}

func ExampleConcurrence() {
	err := jobs.Concurrence{
		func(ctx context.Context) error {
			// concurrent job 1
			return nil
		},
		func(ctx context.Context) error {
			// concurrent job 2
			return nil
		},
	}.Run(context.Background())
	_ = err
}

var _ jobs.Runnable = jobs.Concurrence{}

func TestConcurrence_Run(t *testing.T) {
	var (
		rnd   = random.New(random.CryptoSeed{})
		key   = rnd.String()
		value = rnd.String()
		ctx   = context.WithValue(context.Background(), key, value)
	)

	t.Run("when sequence is uninitialized", func(t *testing.T) {
		var s jobs.Concurrence
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("when every job succeed", func(t *testing.T) {
		s := jobs.Concurrence{
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
		}
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("the jobs are executed", func(t *testing.T) {
		var out int32
		s := jobs.Concurrence{
			func(ctx context.Context) error { atomic.AddInt32(&out, 1); return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 10); return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 100); return nil },
		}
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))

			assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
				it.Must.Equal(int32(111), atomic.LoadInt32(&out))
			})
		})
	})

	t.Run("the .Run will wait until the jobs are done", func(t *testing.T) {
		s := jobs.Concurrence{
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		}
		assert.NotWithin(t, blockCheckWaitTime, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})

	t.Run("the jobs are executed concurrently", func(t *testing.T) {
		var (
			out   int32
			done1 = make(chan struct{})
			done2 = make(chan struct{})
			done3 = make(chan struct{})
		)
		s := jobs.Concurrence{
			func(ctx context.Context) error { atomic.AddInt32(&out, 1); <-done1; return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 10); <-done2; return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 100); <-done3; return nil },
		}

		go func() { assert.NoError(t, s.Run(ctx)) }()
		assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
			it.Must.Equal(int32(111), atomic.LoadInt32(&out))
		})
	})

	t.Run("when a job fails, it will interrupt the rest of the concurrent job and we get back the failure", func(t *testing.T) {
		expErr := rnd.Error()
		s := jobs.Concurrence{
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { return expErr },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		}
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.ErrorIs(t, expErr, s.Run(ctx))
		})
	})

	t.Run("when multiple job fails, then all collected and merged into a single error", func(t *testing.T) {
		var (
			expErr1 = rnd.Error()
			expErr2 = rnd.Error()
			expErr3 = rnd.Error()
		)
		s := jobs.Concurrence{
			func(ctx context.Context) error { return expErr1 },
			func(ctx context.Context) error { return expErr2 },
			func(ctx context.Context) error { return expErr3 },
		}
		assert.Within(t, time.Second, func(ctx context.Context) {
			gotErr := s.Run(ctx)
			assert.ErrorIs(t, expErr1, gotErr)
			assert.ErrorIs(t, expErr2, gotErr)
			assert.ErrorIs(t, expErr3, gotErr)
		})
	})

	t.Run("when job fails with context cancellation, it is not reported back", func(t *testing.T) {
		s := jobs.Concurrence{
			func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
			func(ctx context.Context) error { return context.Canceled },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		}
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})

	t.Run("jobs will receive the input context", func(t *testing.T) {
		assertContext := func(ctx context.Context) {
			assert.NotNil(t, ctx)
			gotValue, ok := ctx.Value(key).(string)
			assert.True(t, ok, "key contains a value")
			assert.Equal(t, value, gotValue, "the received value is what we expect")
		}
		s := jobs.Concurrence{
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
		}
		assert.Within(t, time.Second, func(context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})
}
