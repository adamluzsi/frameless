package tasks_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/tasks"
	"github.com/adamluzsi/frameless/pkg/tasks/internal"
	"github.com/adamluzsi/frameless/pkg/tasks/schedule"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/let"
	"github.com/adamluzsi/testcase/random"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const blockCheckWaitTime = 42 * time.Millisecond

func StubSignalNotify(t *testcase.T, fn func(chan<- os.Signal, ...os.Signal)) {
	var (
		notify = internal.SignalNotify
		stop   = internal.SignalStop
	)
	t.Cleanup(func() {
		internal.SignalNotify = notify
		internal.SignalStop = stop
	})
	internal.SignalNotify = fn
	internal.SignalStop = func(chan<- os.Signal) {}
}

func StubShutdownTimeout(tb testing.TB, timeout time.Duration) {
	og := internal.JobGracefulShutdownTimeout
	tb.Cleanup(func() { internal.JobGracefulShutdownTimeout = og })
	internal.JobGracefulShutdownTimeout = timeout
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var _ tasks.Runnable = (tasks.Task)(nil)

func TestToJob_smoke(t *testing.T) {
	expErr := random.New(random.CryptoSeed{}).Error()
	job := tasks.ToTask(func() error { return expErr })
	assert.NotNil(t, job)
	assert.Equal(t, expErr, job(context.Background()))
}

func TestToJob(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})

	t.Run("on Job", func(t *testing.T) {
		assert.NotNil(t, tasks.ToTask(func(ctx context.Context) error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, tasks.ToTask(func(ctx context.Context) error { return expErr })(context.Background()))
		assert.NoError(t, tasks.ToTask(func(ctx context.Context) error {
			assert.Equal(t, "v", ctx.Value("k").(string))
			return nil
		})(context.WithValue(context.Background(), "k", "v")))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, tasks.ToTask(func() error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, tasks.ToTask(func() error { return expErr })(context.Background()))
	})

	t.Run("on func(context.Context)", func(t *testing.T) {
		assert.NotNil(t, tasks.ToTask(func(ctx context.Context) {}))
		var ran bool
		assert.NoError(t, tasks.ToTask(func(ctx context.Context) { ran = true })(context.Background()))
		assert.True(t, ran)
		assert.NotWithin(t, blockCheckWaitTime, func(ctx context.Context) {
			assert.NoError(t, tasks.ToTask(func(ctx context.Context) { <-ctx.Done() })(ctx))
		})
		type key struct{}
		assert.NoError(t, tasks.ToTask(func(ctx context.Context) {
			assert.Equal(t, any(42), ctx.Value(key{}))
		})(context.WithValue(context.Background(), key{}, 42)))
	})

	t.Run("on func()", func(t *testing.T) {
		var ran bool
		assert.NoError(t, tasks.ToTask(func() { ran = true })(context.Background()))
		assert.True(t, ran)
	})

	t.Run("on *Runnable", func(t *testing.T) {
		expErr := rnd.Error()
		var r tasks.Runnable = tasks.Sequence(func(ctx context.Context) error { return expErr })
		job := tasks.ToTask(&r)
		assert.NotNil(t, job)
		assert.ErrorIs(t, expErr, job(context.Background()))
		type key struct{}
		r = tasks.Sequence(func(ctx context.Context) {
			assert.Equal(t, any(42), ctx.Value(key{}))
		})
		assert.NoError(t, tasks.ToTask(&r)(context.WithValue(context.Background(), key{}, 42)))
	})
}

func ExampleSequence() {
	err := tasks.Sequence(
		func(ctx context.Context) error {
			// first job to execute
			return nil
		},
		func(ctx context.Context) error {
			// follow-up job to execute
			return nil
		},
	).Run(context.Background())
	_ = err
}

var _ tasks.Runnable = tasks.Sequence[func()]()

func TestSequence_Run(t *testing.T) {
	var (
		rnd   = random.New(random.CryptoSeed{})
		key   = rnd.String()
		value = rnd.String()
		ctx   = context.WithValue(context.Background(), key, value)
	)

	t.Run("when sequence is uninitialized", func(t *testing.T) {
		var s = tasks.Sequence[func()]()
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("when every job succeed", func(t *testing.T) {
		s := tasks.Sequence(
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("the tasks in the Sequence are executed in a sequence order", func(t *testing.T) {
		var out []int
		s := tasks.Sequence(
			func(ctx context.Context) error { out = append(out, 1); return nil },
			func(ctx context.Context) error { out = append(out, 2); return nil },
			func(ctx context.Context) error { out = append(out, 3); return nil },
		)
		assert.NoError(t, s.Run(ctx))
		assert.Equal(t, []int{1, 2, 3}, out)
	})

	t.Run("when a job fails, it breaks the sequence and we get back the failure", func(t *testing.T) {
		expErr := rnd.Error()
		var out []int
		s := tasks.Sequence(
			func(ctx context.Context) error { out = append(out, 1); return nil },
			func(ctx context.Context) error { out = append(out, 2); return expErr },
			func(ctx context.Context) error { out = append(out, 3); return nil },
		)
		assert.ErrorIs(t, expErr, s.Run(ctx))
		assert.Equal(t, []int{1, 2}, out)
	})

	t.Run("tasks will receive the input context", func(t *testing.T) {
		assertContext := func(ctx context.Context) {
			assert.NotNil(t, ctx)
			gotValue, ok := ctx.Value(key).(string)
			assert.True(t, ok, "key contains a value")
			assert.Equal(t, value, gotValue, "the received value is what we expect")
		}
		s := tasks.Sequence(
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})
}

func Example_sequenceMixedWithConcurrence() {
	_ = tasks.Sequence(
		tasks.Concurrence(
			func() { /* migration task 1 */ },
			func() { /* migration task 2 */ },
		),
		tasks.Concurrence(
			func() { /* task dependent on migrations */ },
			func() { /* task dependent on migrations */ },
			func() { /* task dependent on migrations */ },
		),
	)(context.Background())
}

func ExampleConcurrence() {
	err := tasks.Concurrence(
		func(ctx context.Context) error {
			// concurrent job 1
			return nil
		},
		func(ctx context.Context) error {
			// concurrent job 2
			return nil
		},
	).Run(context.Background())
	_ = err
}

var _ tasks.Runnable = tasks.Concurrence[func()]()

func TestConcurrence_Run(t *testing.T) {
	var (
		rnd   = random.New(random.CryptoSeed{})
		key   = rnd.String()
		value = rnd.String()
		ctx   = context.WithValue(context.Background(), key, value)
	)

	t.Run("when sequence is uninitialized", func(t *testing.T) {
		var s = tasks.Concurrence[func()]()
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("when every job succeed", func(t *testing.T) {
		s := tasks.Concurrence(
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("the tasks are executed", func(t *testing.T) {
		var out int32
		s := tasks.Concurrence(
			func(ctx context.Context) error { atomic.AddInt32(&out, 1); return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 10); return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 100); return nil },
		)
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))

			assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
				it.Must.Equal(int32(111), atomic.LoadInt32(&out))
			})
		})
	})

	t.Run("the .Run will wait until the tasks are done", func(t *testing.T) {
		s := tasks.Concurrence(
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		)
		assert.NotWithin(t, blockCheckWaitTime, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})

	t.Run("the tasks are executed concurrently", func(t *testing.T) {
		var (
			out   int32
			done1 = make(chan struct{})
			done2 = make(chan struct{})
			done3 = make(chan struct{})
		)
		s := tasks.Concurrence(
			func(ctx context.Context) error { atomic.AddInt32(&out, 1); <-done1; return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 10); <-done2; return nil },
			func(ctx context.Context) error { atomic.AddInt32(&out, 100); <-done3; return nil },
		)

		go func() { assert.NoError(t, s.Run(ctx)) }()
		assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
			it.Must.Equal(int32(111), atomic.LoadInt32(&out))
		})
	})

	t.Run("when a job fails, it will interrupt the rest of the concurrent job and we get back the failure", func(t *testing.T) {
		expErr := rnd.Error()
		s := tasks.Concurrence(
			func(ctx context.Context) error { <-ctx.Done(); return nil },
			func(ctx context.Context) error { return expErr },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		)
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
		s := tasks.Concurrence(
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

	t.Run("when job fails with context cancellation, it is not reported back", func(t *testing.T) {
		s := tasks.Concurrence(
			func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
			func(ctx context.Context) error { return context.Canceled },
			func(ctx context.Context) error { <-ctx.Done(); return nil },
		)
		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})

	t.Run("tasks will receive the input context", func(t *testing.T) {
		assertContext := func(ctx context.Context) {
			assert.NotNil(t, ctx)
			gotValue, ok := ctx.Value(key).(string)
			assert.True(t, ok, "key contains a value")
			assert.Equal(t, value, gotValue, "the received value is what we expect")
		}
		s := tasks.Concurrence(
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
		)
		assert.Within(t, time.Second, func(context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})
}

func Test_Main(t *testing.T) {
	s := testcase.NewSpec(t)

	type ContextWithCancel struct {
		context.Context
		Cancel func()
	}
	var (
		contextCancel = testcase.Let(s, func(t *testcase.T) ContextWithCancel {
			ctx, cancel := context.WithCancel(context.Background())
			t.Defer(cancel)
			return ContextWithCancel{
				Context: ctx,
				Cancel:  cancel,
			}
		}).EagerLoading(s) // Eager Loading required to avoid data race from "go act(t)"
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return contextCancel.Get(t)
		})

		Jobs = testcase.LetValue[[]tasks.Task](s, nil)
	)
	act := func(t *testcase.T) error {
		return tasks.Main(contextCancel.Get(t), Jobs.Get(t)...)
	}

	s.When("no job is provided", func(s *testcase.Spec) {
		Jobs.LetValue(s, nil)

		s.Then("it returns early", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				Context.Set(t, ctx)

				t.Must.NoError(act(t))
			})
		})
	})

	s.When("tasks are provided", func(s *testcase.Spec) {
		othJob := testcase.Let(s, func(t *testcase.T) tasks.Task {
			return func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			}
		})

		jobDone := testcase.LetValue[bool](s, false)
		Jobs.Let(s, func(t *testcase.T) []tasks.Task {
			return []tasks.Task{
				func(ctx context.Context) error {
					<-ctx.Done()
					jobDone.Set(t, true)
					return ctx.Err()
				},
				othJob.Get(t),
			}
		})

		s.Then("it will block", func(t *testcase.T) {
			var done int64
			go func() {
				_ = act(t)
				atomic.AddInt64(&done, 1)
			}()
			assert.Waiter{WaitDuration: time.Millisecond}.Wait()
			t.Must.Equal(int64(0), atomic.LoadInt64(&done))
		})

		s.Then("on context cancellation the block stops", func(t *testcase.T) {
			go func() {
				time.Sleep(time.Millisecond)
				contextCancel.Get(t).Cancel()
			}()

			t.Must.Within(time.Second, func(_ context.Context) {
				t.Must.NoError(act(t))
			})
		})

		s.When("subscribed signal is notified", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				StubSignalNotify(t, func(c chan<- os.Signal, sigs ...os.Signal) {
					t.Must.NotEmpty(sigs)
					go func() { c <- t.Random.SliceElement(sigs).(os.Signal) }()
				})
			})

			s.Then("it will not block but signal shutdown and return without an error", func(t *testcase.T) {
				t.Must.Within(time.Second, func(ctx context.Context) {
					Context.Set(t, ctx)
					t.Must.NoError(act(t))
				})
				t.Must.True(jobDone.Get(t))
			})
		})

		s.When("one of the job finish early", func(s *testcase.Spec) {
			othJob.Let(s, func(t *testcase.T) tasks.Task {
				return func(ctx context.Context) error {
					return nil
				}
			})

			s.Then("it will block and doesn't affect the other tasks", func(t *testcase.T) {
				var done int64
				go func() {
					_ = act(t)
					atomic.AddInt64(&done, 1)
				}()
				assert.Waiter{WaitDuration: time.Millisecond}.Wait()
				t.Must.Equal(int64(0), atomic.LoadInt64(&done))
				t.Must.False(jobDone.Get(t))
			})
		})

		s.When("one of the job encounters an error", func(s *testcase.Spec) {
			expectedErr := let.Error(s)

			othJob.Let(s, func(t *testcase.T) tasks.Task {
				return func(ctx context.Context) error {
					return expectedErr.Get(t)
				}
			})

			s.Then("it will not block but signal shutdown and return all doesn't affect the other tasks", func(t *testcase.T) {
				var done int64
				go func() {
					_ = act(t)
					atomic.AddInt64(&done, 1)
				}()
				assert.Waiter{WaitDuration: time.Millisecond}.Wait()
				t.Must.Equal(int64(1), atomic.LoadInt64(&done))
				t.Must.True(jobDone.Get(t))
			})
		})
	})
}

func Test_Main_smoke(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	key, value := rnd.String(), rnd.String()
	expErr := rnd.Error()

	baseCTX, cancel := context.WithCancel(context.WithValue(context.Background(), key, value))

	var (
		gotErr error
		wg     sync.WaitGroup
	)
	wg.Add(1)
	assert.NotWithin(t, 42*time.Millisecond, func(context.Context) {
		defer wg.Done()
		gotErr = tasks.Main(baseCTX, func(ctx context.Context) error {
			assert.Equal(t, value, ctx.Value(key).(string))
			<-ctx.Done()
			return expErr
		})
	}, "expected to block")

	cancel()
	wg.Wait()
	assert.Equal(t, expErr, gotErr)
}

func ExampleWithShutdown() {
	srv := http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}

	httpServerJob := tasks.WithShutdown(srv.ListenAndServe, srv.Shutdown)
	_ = httpServerJob

	ctx, cancel := context.WithCancel(context.Background())
	// listen to a cancellation signal and then call the cancel func
	// or use ShutdownManager.
	_ = cancel

	if err := httpServerJob(ctx); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func ExampleWithRepeat() {
	job := tasks.WithRepeat(schedule.Interval(time.Second), func(ctx context.Context) error {
		// I'm a short-lived job, and prefer to be constantly executed,
		// Repeat will keep repeating me every second until shutdown is signaled.
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	if err := job(ctx); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func TestWithShutdown_smoke(t *testing.T) {
	StubShutdownTimeout(t, time.Millisecond)
	const (
		expectedKey   = "key"
		expectedValue = "value"
	)
	t.Run("with context", func(t *testing.T) {
		var mux sync.Mutex
		var (
			startBegin, startFinished, stopBegin bool
			stopFinished, stopGraceTimeout       bool
		)
		job := tasks.WithShutdown(func(ctx context.Context) error {
			assert.Equal(t, expectedValue, ctx.Value(expectedKey).(string))

			mux.Lock()
			startBegin = true
			mux.Unlock()

			<-ctx.Done()

			mux.Lock()
			startFinished = true
			mux.Unlock()

			return nil
		}, func(ctx context.Context) error {
			assert.Equal(t, expectedValue, ctx.Value(expectedKey).(string))

			mux.Lock()
			stopBegin = true
			mux.Unlock()

			select {
			case <-ctx.Done():
				t.Error("shutdown context timed out too early, not giving graceful shutdown time")
			case <-time.After(internal.JobGracefulShutdownTimeout / 2):
				mux.Lock()
				stopGraceTimeout = true
				mux.Unlock()
			}

			<-ctx.Done()

			mux.Lock()
			stopFinished = true
			mux.Unlock()

			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		assert.NotWithin(t, blockCheckWaitTime, func(context.Context) { // expected to block
			assert.NoError(t, job(context.WithValue(ctx, expectedKey, expectedValue)))
		})
		assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
			mux.Lock()
			defer mux.Unlock()
			it.Must.True(startBegin)
			it.Must.False(startFinished)
		})

		cancel() // cancel job

		assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
			mux.Lock()
			defer mux.Unlock()
			it.Must.True(startFinished)
			it.Must.True(stopBegin)
			it.Must.True(stopFinished)
			it.Must.True(stopGraceTimeout)
		})
	})

	t.Run("smoke on without context", func(t *testing.T) {
		var mux sync.Mutex
		var (
			startOK bool
			stopOK  bool
		)
		job := tasks.WithShutdown(func() error {
			mux.Lock()
			startOK = true
			mux.Unlock()
			time.Sleep(blockCheckWaitTime)
			return nil
		}, func() error {
			mux.Lock()
			stopOK = true
			mux.Unlock()
			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		assert.NotWithin(t, blockCheckWaitTime, func(context.Context) { // expected to block & ignore assert ctx cancel
			assert.NoError(t, job(context.WithValue(ctx, expectedKey, expectedValue)))
		})

		assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
			mux.Lock()
			defer mux.Unlock()
			it.Must.True(startOK)
			it.Must.False(stopOK)
		})

		cancel() // cancel job

		assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
			mux.Lock()
			defer mux.Unlock()
			it.Must.True(stopOK)
		})
	})

	t.Run("error is propagated back from both StartFn", func(t *testing.T) {
		var expectedErr = random.New(random.CryptoSeed{}).Error()

		job := tasks.WithShutdown(func() error {
			return expectedErr
		}, func() error {
			return nil
		})

		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.ErrorIs(t, expectedErr, job(ctx))
		})
	})

	t.Run("error is propagated back from both StopFn", func(t *testing.T) {
		var expectedErr = random.New(random.CryptoSeed{}).Error()

		job := tasks.WithShutdown(func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}, func() error {
			return expectedErr
		})

		assert.Within(t, time.Second, func(ctx context.Context) {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			assert.ErrorIs(t, expectedErr, job(ctx))
		})
	})
}

func TestWithRepeat_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("A job is being repeated", func(t *testcase.T) {
		var count int32
		var job tasks.Task = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		}

		job = tasks.WithRepeat(schedule.Interval(0), job)

		t.Must.NotWithin(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.NoError(job(ctx))
		})

		t.Eventually(func(t assert.It) {
			t.Must.True(1 < atomic.LoadInt32(&count), "should run more than one times, because the repeat")
		})
	})

	s.Test("interval is taken between runs", func(t *testcase.T) {
		var count int32
		var job tasks.Task = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		}

		job = tasks.WithRepeat(schedule.Interval(time.Hour), job)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		t.Must.NotWithin(blockCheckWaitTime, func(context.Context) {
			t.Should.NoError(job(ctx))
		})

		t.Eventually(func(t assert.It) {
			t.Must.Equal(int32(1), atomic.LoadInt32(&count), "should run at least once before the first interval")
		})

		timecop.Travel(t, time.Hour+time.Minute)

		t.Eventually(func(t assert.It) {
			t.Must.Equal(int32(2), atomic.LoadInt32(&count), "should run at twice because one interval passed")
		})
	})

	s.Test("cancellation is propagated", func(t *testcase.T) {
		var job tasks.Task = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		job = tasks.WithRepeat(schedule.Interval(0), job)

		var done int32
		t.Must.NotWithin(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.NoError(job(ctx))
			atomic.AddInt32(&done, 1)
		})

		t.Eventually(func(t assert.It) {
			const msg = "cancellation was expected to interrupt the wrapped job function"
			t.Must.Equal(int32(1), atomic.LoadInt32(&done), msg)
		})
	})

	s.Test("on error, the error is returned", func(t *testcase.T) {
		expErr := t.Random.Error()

		var count int32
		var job tasks.Task = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return expErr
		}

		job = tasks.WithRepeat(schedule.Interval(0), job)

		t.Must.Within(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.ErrorIs(expErr, job(ctx))
		})

		t.Must.Equal(int32(1), atomic.LoadInt32(&count), "job was expected to run only once")
	})

	s.Test("on error that happens eventually, the error is returned", func(t *testcase.T) {
		expErr := t.Random.Error()

		var count int32
		var job tasks.Task = func(ctx context.Context) error {
			if 1 < atomic.LoadInt32(&count) {
				return expErr
			}
			atomic.AddInt32(&count, 1)
			return nil
		}

		job = tasks.WithRepeat(schedule.Interval(0), job)

		t.Must.Within(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.ErrorIs(expErr, job(ctx))
		})
	})
}

func ExampleOnError() {
	jobWithErrorHandling := tasks.OnError(
		func(ctx context.Context) error { return nil },                          // job
		func(err error) error { log.Println("ERROR", err.Error()); return nil }, // error handling
	)
	_ = jobWithErrorHandling
}
func TestOnError(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("on no error, error handler is not triggered", func(t *testcase.T) {
		job := tasks.OnError(func() error { return nil }, func(err error) error { panic("boom") })
		t.Must.NoError(job(context.Background()))
	})

	s.Test("on context cancellation, error handler is not triggered", func(t *testcase.T) {
		job := tasks.OnError(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}, func(err error) error { panic("boom") })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		t.Must.Equal(ctx.Err(), job(ctx))
	})

	s.Test("on non context related error, error is propagated to the error handler", func(t *testcase.T) {
		var (
			expErrIn  = t.Random.Error()
			expErrOut = t.Random.Error()
			gotErrIn  error
		)
		job := tasks.OnError(func(ctx context.Context) error {
			return expErrIn
		}, func(err error) error {
			gotErrIn = err
			return expErrOut
		})
		t.Must.Equal(expErrOut, job(context.Background()))
		t.Must.Equal(expErrIn, gotErrIn)
	})
}

func ExampleWithSignalNotify() {
	srv := http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}

	job := tasks.WithShutdown(srv.ListenAndServe, srv.Shutdown)
	job = tasks.WithSignalNotify(job)

	if err := job(context.Background()); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func TestWithSignalNotify(t *testing.T) {
	s := testcase.NewSpec(t)

	type ContextWithCancel struct {
		context.Context
		Cancel func()
	}
	var (
		contextCancel = testcase.Let(s, func(t *testcase.T) ContextWithCancel {
			ctx, cancel := context.WithCancel(context.Background())
			t.Defer(cancel)
			return ContextWithCancel{
				Context: ctx,
				Cancel:  cancel,
			}
		}).EagerLoading(s) // Eager Loading required to avoid data race from "go act(t)"
		Context = testcase.Let(s, func(t *testcase.T) context.Context {
			return contextCancel.Get(t)
		})
	)
	var (
		jobDone = testcase.LetValue[bool](s, false)
		job     = testcase.Let(s, func(t *testcase.T) tasks.Task {
			return func(ctx context.Context) error {
				<-ctx.Done()
				jobDone.Set(t, true)
				return ctx.Err()
			}
		})
		signals = testcase.LetValue[[]os.Signal](s, nil)
	)
	act := func(t *testcase.T) error {
		return tasks.WithSignalNotify(job.Get(t), signals.Get(t)...)(Context.Get(t))
	}

	s.When("signal is provided", func(s *testcase.Spec) {
		signals.Let(s, func(t *testcase.T) []os.Signal {
			return []os.Signal{syscall.Signal(42)}
		})

		s.Then("it will use the signals to subscribe for notify", func(t *testcase.T) {
			var run bool
			StubSignalNotify(t, func(c chan<- os.Signal, sigs ...os.Signal) {
				run = true
				t.Must.ContainExactly(signals.Get(t), sigs)
			})

			t.Must.NotWithin(time.Second, func(context.Context) {
				t.Must.NoError(act(t))
			})

			t.Must.True(run)
		})
	})

	s.Then("it will block", func(t *testcase.T) {
		var done int64
		go func() {
			_ = act(t)
			atomic.AddInt64(&done, 1)
		}()
		assert.Waiter{WaitDuration: time.Millisecond}.Wait()
		t.Must.Equal(int64(0), atomic.LoadInt64(&done))
	})

	s.Then("on context cancellation the block stops", func(t *testcase.T) {
		go func() {
			time.Sleep(time.Millisecond)
			contextCancel.Get(t).Cancel()
		}()

		t.Must.Within(time.Second, func(_ context.Context) {
			t.Must.NoError(act(t))
		})
	})

	s.When("subscribed signal is notified", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			StubSignalNotify(t, func(c chan<- os.Signal, sigs ...os.Signal) {
				t.Must.NotEmpty(sigs)
				go func() { c <- t.Random.SliceElement(sigs).(os.Signal) }()
			})
		})

		s.Then("it will not block but signal shutdown and return without an error", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				Context.Set(t, ctx)
				t.Must.NoError(act(t))
			})
			t.Must.True(jobDone.Get(t))
		})
	})

	s.When("the job finish early", func(s *testcase.Spec) {
		job.Let(s, func(t *testcase.T) tasks.Task {
			return func(ctx context.Context) error {
				return nil
			}
		})

		s.Then("it returns early", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				t.Must.NoError(act(t))
			})
		})
	})

	s.When("the job encounters an error", func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		job.Let(s, func(t *testcase.T) tasks.Task {
			return func(ctx context.Context) error {
				return expectedErr.Get(t)
			}
		})

		s.Then("error is returned", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				t.Must.ErrorIs(expectedErr.Get(t), act(t))
			})
		})
	})
}
