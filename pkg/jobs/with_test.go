package jobs_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/jobs"
	"github.com/adamluzsi/frameless/pkg/jobs/internal"
	"github.com/adamluzsi/frameless/pkg/jobs/schedule"
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

func ExampleWithShutdown() {
	srv := http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}

	httpServerJob := jobs.WithShutdown(srv.ListenAndServe, srv.Shutdown)
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
	job := jobs.WithRepeat(schedule.Interval(time.Second), func(ctx context.Context) error {
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
		job := jobs.WithShutdown(func(ctx context.Context) error {
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
		job := jobs.WithShutdown(func() error {
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

		job := jobs.WithShutdown(func() error {
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

		job := jobs.WithShutdown(func(ctx context.Context) error {
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
		var job jobs.Job = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		}

		job = jobs.WithRepeat(schedule.Interval(0), job)

		t.Must.NotWithin(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.NoError(job(ctx))
		})

		t.Eventually(func(t assert.It) {
			t.Must.True(1 < atomic.LoadInt32(&count), "should run more than one times, because the repeat")
		})
	})

	s.Test("interval is taken between runs", func(t *testcase.T) {
		var count int32
		var job jobs.Job = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		}

		job = jobs.WithRepeat(schedule.Interval(time.Hour), job)

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
		var job jobs.Job = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		job = jobs.WithRepeat(schedule.Interval(0), job)

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
		var job jobs.Job = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return expErr
		}

		job = jobs.WithRepeat(schedule.Interval(0), job)

		t.Must.Within(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.ErrorIs(expErr, job(ctx))
		})

		t.Must.Equal(int32(1), atomic.LoadInt32(&count), "job was expected to run only once")
	})

	s.Test("on error that happens eventually, the error is returned", func(t *testcase.T) {
		expErr := t.Random.Error()

		var count int32
		var job jobs.Job = func(ctx context.Context) error {
			if 1 < atomic.LoadInt32(&count) {
				return expErr
			}
			atomic.AddInt32(&count, 1)
			return nil
		}

		job = jobs.WithRepeat(schedule.Interval(0), job)

		t.Must.Within(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.ErrorIs(expErr, job(ctx))
		})
	})
}

func ExampleOnError() {
	jobWithErrorHandling := jobs.OnError(
		func(ctx context.Context) error { return nil },                          // job
		func(err error) error { log.Println("ERROR", err.Error()); return nil }, // error handling
	)
	_ = jobWithErrorHandling
}
func TestOnError(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("on no error, error handler is not triggered", func(t *testcase.T) {
		job := jobs.OnError(func() error { return nil }, func(err error) error { panic("boom") })
		t.Must.NoError(job(context.Background()))
	})

	s.Test("on context cancellation, error handler is not triggered", func(t *testcase.T) {
		job := jobs.OnError(func(ctx context.Context) error {
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
		job := jobs.OnError(func(ctx context.Context) error {
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

	job := jobs.WithShutdown(srv.ListenAndServe, srv.Shutdown)
	job = jobs.WithSignalNotify(job)

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
		job     = testcase.Let(s, func(t *testcase.T) jobs.Job {
			return func(ctx context.Context) error {
				<-ctx.Done()
				jobDone.Set(t, true)
				return ctx.Err()
			}
		})
		signals = testcase.LetValue[[]os.Signal](s, nil)
	)
	act := func(t *testcase.T) error {
		return jobs.WithSignalNotify(job.Get(t), signals.Get(t)...)(Context.Get(t))
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
		job.Let(s, func(t *testcase.T) jobs.Job {
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

		job.Let(s, func(t *testcase.T) jobs.Job {
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
