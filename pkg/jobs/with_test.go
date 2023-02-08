package jobs_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/jobs"
	"github.com/adamluzsi/frameless/pkg/jobs/internal"
	"github.com/adamluzsi/frameless/pkg/jobs/schedule"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/clock/timecop"
	"github.com/adamluzsi/testcase/random"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
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
