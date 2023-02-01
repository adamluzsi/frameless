package sysutil_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/sysutil"
	"github.com/adamluzsi/frameless/pkg/sysutil/internal"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"
)

func ExampleJobWithShutdown() {
	srv := http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}

	httpServerJob := sysutil.JobWithShutdown(srv.ListenAndServe, srv.Shutdown)
	_ = httpServerJob

	ctx, cancel := context.WithCancel(context.Background())
	// listen to a cancellation signal and then call the cancel func
	// or use ShutdownManager.
	_ = cancel

	if err := httpServerJob(ctx); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func StubShutdownTimeout(tb testing.TB, timeout time.Duration) {
	og := internal.JobGracefulShutdownTimeout
	tb.Cleanup(func() { internal.JobGracefulShutdownTimeout = og })
	internal.JobGracefulShutdownTimeout = timeout
}

func TestJobWithShutdown_smoke(t *testing.T) {
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
		job := sysutil.JobWithShutdown(func(ctx context.Context) error {
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

		assert.NotWithin(t, 42*time.Millisecond, func(context.Context) { // expected to block
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
		job := sysutil.JobWithShutdown(func() error {
			mux.Lock()
			startOK = true
			mux.Unlock()
			time.Sleep(42 * time.Millisecond)
			return nil
		}, func() error {
			mux.Lock()
			stopOK = true
			mux.Unlock()
			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		assert.NotWithin(t, time.Microsecond, func(context.Context) { // expected to block & ignore assert ctx cancel
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

		job := sysutil.JobWithShutdown(func() error {
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

		job := sysutil.JobWithShutdown(func(ctx context.Context) error {
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
