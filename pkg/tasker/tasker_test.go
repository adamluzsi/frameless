package tasker_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/internal/signalint"
	"go.llib.dev/frameless/pkg/internal/taskerlite"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/tasker/internal"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

var _ = tasker.Task((taskerlite.Task)(nil))
var _ tasker.Runnable = (taskerlite.Task)(nil)

func StubSignalNotify(t *testcase.T, fn func(chan<- os.Signal, ...os.Signal)) {
	t.Cleanup(signalint.StubNotify(fn))
	t.Cleanup(signalint.StubStop(func(chan<- os.Signal) {}))
}

func StubShutdownTimeout(tb testing.TB, timeout time.Duration) {
	og := internal.GracefulShutdownTimeout
	tb.Cleanup(func() { internal.GracefulShutdownTimeout = og })
	internal.GracefulShutdownTimeout = timeout
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var _ tasker.Runnable = (tasker.Task)(nil)

func TestToTask_smoke(t *testing.T) {
	expErr := random.New(random.CryptoSeed{}).Error()
	task := tasker.ToTask(func() error { return expErr })
	assert.NotNil(t, task)
	assert.Equal(t, expErr, task(context.Background()))
}

func TestToTask(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})

	t.Run("on Task", func(t *testing.T) {
		assert.NotNil(t, tasker.ToTask(func(ctx context.Context) error { return nil }))
		expErr := rnd.Error()
		type key struct{}
		val := rnd.String()
		assert.Equal(t, expErr, tasker.ToTask(func(ctx context.Context) error { return expErr })(context.Background()))
		assert.NoError(t, tasker.ToTask(func(ctx context.Context) error {
			assert.Equal(t, val, ctx.Value(key{}).(string))
			return nil
		})(context.WithValue(context.Background(), key{}, val)))
	})

	t.Run("on func() error", func(t *testing.T) {
		assert.NotNil(t, tasker.ToTask(func() error { return nil }))
		expErr := rnd.Error()
		assert.Equal(t, expErr, tasker.ToTask(func() error { return expErr })(context.Background()))
	})

	t.Run("on func(context.Context)", func(t *testing.T) {
		assert.NotNil(t, tasker.ToTask(func(ctx context.Context) {}))
		var ran bool
		assert.NoError(t, tasker.ToTask(func(ctx context.Context) { ran = true })(context.Background()))
		assert.True(t, ran)
		assert.NotWithin(t, blockCheckWaitTime, func(ctx context.Context) {
			assert.NoError(t, tasker.ToTask(func(ctx context.Context) { <-ctx.Done() })(ctx))
		})
		type key struct{}
		assert.NoError(t, tasker.ToTask(func(ctx context.Context) {
			assert.Equal(t, any(42), ctx.Value(key{}))
		})(context.WithValue(context.Background(), key{}, 42)))
	})

	t.Run("on func()", func(t *testing.T) {
		var ran bool
		assert.NoError(t, tasker.ToTask(func() { ran = true })(context.Background()))
		assert.True(t, ran)
	})

	t.Run("on *Runnable", func(t *testing.T) {
		expErr := rnd.Error()
		var r tasker.Runnable = tasker.Sequence(func(ctx context.Context) error { return expErr })
		task := tasker.ToTask(&r)
		assert.NotNil(t, task)
		assert.ErrorIs(t, expErr, task(context.Background()))
		type key struct{}
		r = tasker.Sequence(func(ctx context.Context) {
			assert.Equal(t, any(42), ctx.Value(key{}))
		})
		assert.NoError(t, tasker.ToTask(&r)(context.WithValue(context.Background(), key{}, 42)))
	})
}

func ExampleSequence() {
	err := tasker.Sequence(
		func(ctx context.Context) error {
			// first task to execute
			return nil
		},
		func(ctx context.Context) error {
			// follow-up task to execute
			return nil
		},
	).Run(context.Background())
	_ = err
}

var _ tasker.Runnable = tasker.Sequence[func()]()

func TestSequence_Run(t *testing.T) {
	type Key struct{ V string }

	var (
		rnd   = random.New(random.CryptoSeed{})
		key   = Key{V: rnd.String()}
		value = rnd.String()
		ctx   = context.WithValue(context.Background(), key, value)
	)

	t.Run("when sequence is uninitialized", func(t *testing.T) {
		var s = tasker.Sequence[func()]()
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("when every task succeed", func(t *testing.T) {
		s := tasker.Sequence(
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("the tasker in the Sequence are executed in a sequence order", func(t *testing.T) {
		var out []int
		s := tasker.Sequence(
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
		s := tasker.Sequence(
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
		s := tasker.Sequence(
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})
}

func Example_sequenceMixedWithConcurrence() {
	_ = tasker.Sequence(
		tasker.Concurrence(
			func() { /* migration task 1 */ },
			func() { /* migration task 2 */ },
		),
		tasker.Concurrence(
			func() { /* task dependent on migrations */ },
			func() { /* task dependent on migrations */ },
			func() { /* task dependent on migrations */ },
		),
	)(context.Background())
}

func ExampleConcurrence() {
	err := tasker.Concurrence(
		func(ctx context.Context) error {
			// concurrent task 1
			return nil
		},
		func(ctx context.Context) error {
			// concurrent task 2
			return nil
		},
	).Run(context.Background())
	_ = err
}

var _ tasker.Runnable = tasker.Concurrence[func()]()

func TestConcurrence_Run(t *testing.T) {
	type Key struct{ V string }

	var (
		rnd   = random.New(random.CryptoSeed{})
		key   = Key{V: rnd.String()}
		value = rnd.String()
		ctx   = context.WithValue(context.Background(), key, value)
	)

	t.Run("when sequence is uninitialized", func(t *testing.T) {
		var s = tasker.Concurrence[func()]()
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("when every task succeed", func(t *testing.T) {
		s := tasker.Concurrence(
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
			func(ctx context.Context) error { return nil },
		)
		assert.NoError(t, s.Run(ctx))
	})

	t.Run("the tasker are executed", func(t *testing.T) {
		var out int32
		s := tasker.Concurrence(
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
		s := tasker.Concurrence(
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
		s := tasker.Concurrence(
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
		s := tasker.Concurrence(
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
		s := tasker.Concurrence(
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
		s := tasker.Concurrence(
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
		s := tasker.Concurrence(
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
			func(ctx context.Context) error { assertContext(ctx); return nil },
		)
		assert.Within(t, time.Second, func(context.Context) {
			assert.NoError(t, s.Run(ctx))
		})
	})
}

func ExampleMain_withRunnable() {
	var (
		ctx  = context.Background()
		task tasker.Runnable
	)
	if err := tasker.Main(ctx, &task); err != nil {
		logger.Fatal(ctx, "error in main", logging.ErrField(err))
		os.Exit(1)
	}
}

func ExampleMain_withTask() {
	var (
		ctx  = context.Background()
		task tasker.Task
	)
	if err := tasker.Main(ctx, task); err != nil {
		logger.Fatal(ctx, "error in main", logging.ErrField(err))
		os.Exit(1)
	}
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

		Tasks = testcase.LetValue[[]tasker.Task](s, nil)
	)
	act := func(t *testcase.T) error {
		return tasker.Main(contextCancel.Get(t), Tasks.Get(t)...)
	}

	s.When("no task is provided", func(s *testcase.Spec) {
		Tasks.LetValue(s, nil)

		s.Then("it returns early", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				Context.Set(t, ctx)

				t.Must.NoError(act(t))
			})
		})
	})

	s.When("tasker are provided", func(s *testcase.Spec) {
		othTask := testcase.Let(s, func(t *testcase.T) tasker.Task {
			return func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			}
		})

		isDone := testcase.LetValue[bool](s, false)
		Tasks.Let(s, func(t *testcase.T) []tasker.Task {
			return []tasker.Task{
				func(ctx context.Context) error {
					<-ctx.Done()
					isDone.Set(t, true)
					return ctx.Err()
				},
				othTask.Get(t),
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
					go func() { c <- t.Random.Pick(sigs).(os.Signal) }()
				})
			})

			s.Then("it will not block but signal shutdown and return without an error", func(t *testcase.T) {
				t.Must.Within(time.Second, func(ctx context.Context) {
					Context.Set(t, ctx)
					t.Must.NoError(act(t))
				})
				t.Must.True(isDone.Get(t))
			})
		})

		s.When("one of the task finish early", func(s *testcase.Spec) {
			othTask.Let(s, func(t *testcase.T) tasker.Task {
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
				t.Must.False(isDone.Get(t))
			})
		})

		s.When("one of the task encounters an error", func(s *testcase.Spec) {
			expectedErr := let.Error(s)

			othTask.Let(s, func(t *testcase.T) tasker.Task {
				return func(ctx context.Context) error {
					return expectedErr.Get(t)
				}
			})

			s.Then("it will not block but signal shutdown and return all doesn't affect the other tasker", func(t *testcase.T) {
				var done int64
				var wg sync.WaitGroup
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = act(t)
					atomic.AddInt64(&done, 1)
				}()
				wg.Wait()

				t.Must.Equal(int64(1), atomic.LoadInt64(&done))
				t.Must.True(isDone.Get(t))
			})
		})
	})
}

func Test_Main_contextCancelDoesNotBubbleUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var (
		wg         sync.WaitGroup
		mainErrOut = make(chan error)
		ready      int32
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		mainErrOut <- tasker.Main(ctx, func(ctx context.Context) error {
			atomic.AddInt32(&ready, 1)
			<-ctx.Done()
			return fmt.Errorf("cancelled: %w", ctx.Err())
		})
	}()

	assert.Eventually(t, time.Second, func(it testing.TB) {
		assert.Equal(it, atomic.LoadInt32(&ready), int32(1))
	})

	cancel()

	gotErr, ok := <-mainErrOut
	assert.True(t, ok)
	assert.NoError(t, gotErr)

	wg.Wait()
}

func Test_Main_smoke(t *testing.T) {
	type Key struct{ V string }
	rnd := random.New(random.CryptoSeed{})
	key := Key{V: rnd.String()}
	value := rnd.String()
	expErr := rnd.Error()

	baseCTX, cancel := context.WithCancel(context.WithValue(context.Background(), key, value))

	var (
		gotErr error
		wg     sync.WaitGroup
	)
	wg.Add(1)
	assert.NotWithin(t, 42*time.Millisecond, func(context.Context) {
		defer wg.Done()
		gotErr = tasker.Main(baseCTX, func(ctx context.Context) error {
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
	task := tasker.WithShutdown(
		func(ctx context.Context) error {
			// start working
			<-ctx.Done()
			return nil
		},
		func(ctx context.Context) error {
			// graceful stop for work
			<-ctx.Done()
			return nil
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	// listen to a cancellation signal and then call the cancel func
	// or use ShutdownManager.
	_ = cancel

	if err := task(ctx); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func ExampleWithShutdown_httpServer() {
	srv := http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}
	httpServerTask := tasker.WithShutdown(
		tasker.IgnoreError(srv.ListenAndServe, http.ErrServerClosed),
		srv.Shutdown,
	)

	if err := tasker.WithSignalNotify(httpServerTask)(context.Background()); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func ExampleWithRepeat() {
	task := tasker.WithRepeat(tasker.Every(time.Second), func(ctx context.Context) error {
		// I'm a short-lived task, and prefer to be constantly executed,
		// Repeat will keep repeating me every second until shutdown is signaled.
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	if err := task(ctx); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func TestWithShutdown(t *testing.T) { // TODO: flaky
	StubShutdownTimeout(t, time.Millisecond)
	s := testcase.NewSpec(t)

	type Key struct{}
	var expectedKey = Key{}
	const expectedValue = "value"

	s.Test("with context", func(t *testcase.T) {
		var (
			startBegin, startFinished, stopBegin int32
			stopFinished, stopGraceTimeout       int32
		)
		task := tasker.WithShutdown(func(ctx context.Context) error {
			assert.Equal(t, expectedValue, ctx.Value(expectedKey).(string))
			atomic.AddInt32(&startBegin, 1)
			<-ctx.Done()
			atomic.AddInt32(&startFinished, 1)
			return nil
		}, func(ctx context.Context) error {
			assert.Equal(t, expectedValue, ctx.Value(expectedKey).(string))
			atomic.AddInt32(&stopBegin, 1)
			select {
			case <-ctx.Done():
				t.Error("shutdown context timed out too early, not giving graceful shutdown time")
			case <-time.After(internal.GracefulShutdownTimeout / 2):
				atomic.AddInt32(&stopGraceTimeout, 1)
			}
			<-ctx.Done()
			atomic.AddInt32(&stopFinished, 1)
			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		assert.NotWithin(t, blockCheckWaitTime, func(context.Context) { // expected to block
			assert.NoError(t, task(context.WithValue(ctx, expectedKey, expectedValue)))
		})
		assert.Eventually(t, time.Second, func(it testing.TB) {
			assert.True(it, atomic.LoadInt32(&startBegin) == 1)
			assert.True(it, atomic.LoadInt32(&startFinished) == 0)
		})

		cancel() // cancel task

		assert.Eventually(t, time.Second+500*time.Millisecond, func(it testing.TB) {
			assert.True(it, atomic.LoadInt32(&startFinished) == 1)
			assert.True(it, atomic.LoadInt32(&stopBegin) == 1)
			assert.True(it, atomic.LoadInt32(&stopFinished) == 1)
			assert.True(it, atomic.LoadInt32(&stopGraceTimeout) == 1)
		})
	})

	s.Test("smoke on without context", func(t *testcase.T) {
		var (
			startOK int32
			stopOK  int32
			done    = make(chan struct{})
		)
		task := tasker.WithShutdown(func() error {
			atomic.StoreInt32(&startOK, 1)
			<-done
			return nil
		}, func() error {
			atomic.StoreInt32(&stopOK, 1)
			return nil
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		w := assert.NotWithin(t, blockCheckWaitTime, func(context.Context) { // expected to block & ignore assert ctx cancel
			ctx := context.WithValue(ctx, expectedKey, expectedValue)
			err := task(ctx)
			assert.NoError(t, err)
		})

		assert.Eventually(t, time.Second, func(it testing.TB) {
			assert.True(it, atomic.LoadInt32(&startOK) == 1, "it was expected that task started")
			assert.True(it, atomic.LoadInt32(&stopOK) == 0, "it was not expected that the task already stopped")
		})

		close(done) // cancel StartTask
		cancel()    // cancel task all together
		w.Wait()

		assert.Eventually(t, time.Second, func(it testing.TB) {
			assert.True(it, atomic.LoadInt32(&stopOK) == 1)
		})
	})

	s.Test("error is propagated back from both StartFn", func(t *testcase.T) {
		var expectedErr = random.New(random.CryptoSeed{}).Error()

		task := tasker.WithShutdown(func() error {
			return expectedErr
		}, func() error {
			return nil
		})

		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.ErrorIs(t, expectedErr, task(ctx))
		})
	})

	s.Test("error is propagated back from both StopFn", func(t *testcase.T) {
		var expectedErr = random.New(random.CryptoSeed{}).Error()

		task := tasker.WithShutdown(func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}, func() error {
			return expectedErr
		})

		assert.Within(t, time.Second, func(ctx context.Context) {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			assert.ErrorIs(t, expectedErr, task(ctx))
		})
	})
}

func TestWithRepeat_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("A task is being repeated", func(t *testcase.T) {
		var count int32
		var task tasker.Task = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		}

		task = tasker.WithRepeat(tasker.Every(0), task)

		t.Must.NotWithin(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.NoError(task(ctx))
		})

		t.Eventually(func(t *testcase.T) {
			t.Must.True(1 < atomic.LoadInt32(&count), "should run more than one times, because the repeat")
		})
	})

	s.Test("interval is taken between runs", func(t *testcase.T) {
		var count int32
		var task tasker.Task = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return nil
		}

		task = tasker.WithRepeat(tasker.Every(time.Hour), task)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		t.Must.NotWithin(blockCheckWaitTime, func(context.Context) {
			t.Should.NoError(task(ctx))
		})

		t.Eventually(func(t *testcase.T) {
			t.Must.Equal(int32(1), atomic.LoadInt32(&count), "should run at least once before the first interval")
		})

		timecop.Travel(t, time.Hour+time.Minute)

		t.Eventually(func(t *testcase.T) {
			t.Must.Equal(int32(2), atomic.LoadInt32(&count), "should run at twice because one interval passed")
		})
	})

	s.Test("cancellation is propagated", func(t *testcase.T) {
		var task tasker.Task = func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		}

		task = tasker.WithRepeat(tasker.Every(0), task)

		var done int32
		t.Must.NotWithin(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.NoError(task(ctx))
			atomic.AddInt32(&done, 1)
		})

		t.Eventually(func(t *testcase.T) {
			const msg = "cancellation was expected to interrupt the wrapped task function"
			t.Must.Equal(int32(1), atomic.LoadInt32(&done), msg)
		})
	})

	s.Test("on error, the error is returned", func(t *testcase.T) {
		expErr := t.Random.Error()

		var count int32
		var task tasker.Task = func(ctx context.Context) error {
			atomic.AddInt32(&count, 1)
			return expErr
		}

		task = tasker.WithRepeat(tasker.Every(0), task)

		t.Must.Within(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.ErrorIs(expErr, task(ctx))
		})

		t.Must.Equal(int32(1), atomic.LoadInt32(&count), "task was expected to run only once")
	})

	s.Test("on error that happens eventually, the error is returned", func(t *testcase.T) {
		expErr := t.Random.Error()

		var count int32
		var task tasker.Task = func(ctx context.Context) error {
			if 1 < atomic.LoadInt32(&count) {
				return expErr
			}
			atomic.AddInt32(&count, 1)
			return nil
		}

		task = tasker.WithRepeat(tasker.Every(0), task)

		t.Must.Within(blockCheckWaitTime, func(ctx context.Context) {
			t.Should.ErrorIs(expErr, task(ctx))
		})
	})
}

func ExampleOnError() {
	withErrorHandling := tasker.OnError(
		func(ctx context.Context) error { return nil },                                            // task
		func(ctx context.Context, err error) error { logger.Error(ctx, err.Error()); return nil }, // error handling
	)
	_ = withErrorHandling
}

func TestOnError(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("on no error, error handler is not triggered", func(t *testcase.T) {
		task := tasker.OnError(func() error { return nil }, func(err error) error { panic("boom") })
		t.Must.NoError(task(context.Background()))
	})

	s.Test("on context cancellation, error handler is not triggered", func(t *testcase.T) {
		task := tasker.OnError(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}, func(err error) error { panic("boom") })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		t.Must.Equal(ctx.Err(), task(ctx))
	})

	s.Test("on non context related error, error is propagated to the error handler", func(t *testcase.T) {
		var (
			expErrIn  = t.Random.Error()
			expErrOut = t.Random.Error()
			gotErrIn  error
		)
		task := tasker.OnError(func(ctx context.Context) error {
			return expErrIn
		}, func(err error) error {
			gotErrIn = err
			return expErrOut
		})
		t.Must.Equal(expErrOut, task(context.Background()))
		t.Must.Equal(expErrIn, gotErrIn)
	})

	s.Test("with error handler that accepts context", func(t *testcase.T) {
		var (
			expErrIn  = t.Random.Error()
			expErrOut = t.Random.Error()
			gotErrIn  error
		)
		type ctxKey struct{}
		task := tasker.OnError(func(ctx context.Context) error {
			t.Must.Equal(any(42), ctx.Value(ctxKey{}))
			return expErrIn
		}, func(ctx context.Context, err error) error {
			t.Must.Equal(any(42), ctx.Value(ctxKey{}))
			gotErrIn = err
			return expErrOut
		})
		t.Must.Equal(expErrOut, task(context.WithValue(context.Background(), ctxKey{}, any(42))))
		t.Must.Equal(expErrIn, gotErrIn)
	})
}

func TestIgnoreError_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	type Key struct{}

	s.Test("will wrap the passed task function", func(t *testcase.T) {
		var ran bool
		task := tasker.IgnoreError(func(ctx context.Context) error {
			ran = true
			t.Must.Equal(any(42), ctx.Value(Key{}))
			return nil
		})
		t.Must.NoError(task.Run(context.WithValue(context.Background(), Key{}, any(42))))
		t.Must.True(ran)
	})

	s.Test("on empty error list, all error is ignored", func(t *testcase.T) {
		task := tasker.IgnoreError(func(ctx context.Context) error {
			return t.Random.Error()
		})
		t.Must.NoError(task.Run(context.Background()))
	})

	s.Test("when errors are specified, only they will be ignored", func(t *testcase.T) {
		errToIgnore := t.Random.Error()
		var othErr error
		for {
			othErr = t.Random.Error()
			if errToIgnore != othErr {
				break
			}
		}
		task1 := tasker.IgnoreError(func(ctx context.Context) error {
			return othErr
		}, errToIgnore)
		t.Must.ErrorIs(othErr, task1.Run(context.Background()))
		task2 := tasker.IgnoreError(func(ctx context.Context) error {
			return errToIgnore
		}, errToIgnore)
		t.Must.NoError(task2.Run(context.Background()))
	})
	s.Test("when specified error retruned as wrapped error", func(t *testcase.T) {
		errToIgnore := t.Random.Error()
		task := tasker.IgnoreError(func(ctx context.Context) error {
			return fmt.Errorf("wrapped error: %w", errToIgnore)
		}, errToIgnore)
		t.Must.NoError(task.Run(context.Background()))
	})
}

func ExampleWithSignalNotify() {
	srv := http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}

	task := tasker.WithShutdown(srv.ListenAndServe, srv.Shutdown)
	task = tasker.WithSignalNotify(task)

	if err := task(context.Background()); err != nil {
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
		isDone = testcase.LetValue[bool](s, false)
		task   = testcase.Let(s, func(t *testcase.T) tasker.Task {
			return func(ctx context.Context) error {
				<-ctx.Done()
				isDone.Set(t, true)
				return ctx.Err()
			}
		})
		signals = testcase.LetValue[[]os.Signal](s, nil)
	)
	act := func(t *testcase.T) error {
		return tasker.WithSignalNotify(task.Get(t), signals.Get(t)...)(Context.Get(t))
	}

	s.When("signal is provided", func(s *testcase.Spec) {
		signals.Let(s, func(t *testcase.T) []os.Signal {
			return []os.Signal{syscall.Signal(42)}
		})

		s.Then("it will use the signals to subscribe for notify", func(t *testcase.T) {
			var run atomic.Bool
			StubSignalNotify(t, func(c chan<- os.Signal, sigs ...os.Signal) {
				run.Store(true)
				t.Must.ContainsExactly(signals.Get(t), sigs)
			})

			t.Must.NotWithin(time.Second, func(context.Context) {
				t.Must.NoError(act(t))
			})

			t.Must.True(run.Load())
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
				go func() { c <- t.Random.Pick(sigs).(os.Signal) }()
			})
		})

		s.Then("it will not block but signal shutdown and return without an error", func(t *testcase.T) {
			t.Must.Within(time.Second, func(ctx context.Context) {
				Context.Set(t, ctx)
				t.Must.NoError(act(t))
			})
			t.Must.True(isDone.Get(t))
		})
	})

	s.When("the task finish early", func(s *testcase.Spec) {
		task.Let(s, func(t *testcase.T) tasker.Task {
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

	s.When("the task encounters an error", func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		task.Let(s, func(t *testcase.T) tasker.Task {
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

func TestJobGroup_race(t *testing.T) {
	var g tasker.JobGroup[tasker.FireAndForget]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testcase.Race(func() {
		g.Background(ctx, func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
	}, func() {
		g.Background(ctx, func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
	}, func() {
		g.Go(func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		})
	})
}

func TestJobGroup_wManual_cleanup(t *testing.T) {
	t.Run("group level cleanup", func(t *testing.T) {
		var g tasker.JobGroup[tasker.FireAndForget]

		g.Go(func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		})

		assert.Equal(t, 1, g.Len())

		g.Stop()

		assert.Equal(t, 0, g.Len())
	})

}

func TestJob_Join_safe(t *testing.T) {
	var job tasker.Job

	var expErr = rnd.Error()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, job.Start(ctx, func(ctx context.Context) error {
		<-ctx.Done()
		return expErr
	}))

	var ok int32

	raceBlock := func() {
		assert.Should(t).ErrorIs(job.Join(), expErr)
		atomic.AddInt32(&ok, 1)
	}

	testcase.Race(raceBlock, raceBlock, raceBlock, cancel)

	assert.Equal(t, 3, atomic.LoadInt32(&ok))
}

func TestJobGroup_asFireAndForget(t *testing.T) {
	t.Run("finished jobs are garbage collected", func(t *testing.T) {
		var g = tasker.JobGroup[tasker.FireAndForget]{}

		ctx := context.Background()

		done := make(chan struct{})

		n := rnd.Repeat(3, 7, func() {
			g.Background(ctx, func(ctx context.Context) error {
				<-done
				return nil
			})
		})

		assert.Eventually(t, time.Second, func(t testing.TB) {
			assert.Equal(t, n, g.Len())
		})

		close(done)

		assert.Eventually(t, time.Second, func(t testing.TB) {
			assert.Equal(t, g.Len(), 0)
		})

		assert.Within(t, time.Second/4, func(ctx context.Context) {
			g.Join()
		})
	})
	t.Run("error is not returned back", func(t *testing.T) {
		var g = tasker.JobGroup[tasker.FireAndForget]{}

		ctx := context.Background()

		done := make(chan struct{})

		rnd.Repeat(3, 7, func() {
			g.Background(ctx, func(ctx context.Context) error {
				<-done
				return rnd.Error()
			})
		})

		close(done)

		random.Pick(rnd,
			func() {
				t.Log("Join should wait for the jobs to finish")
				assert.NoError(t, g.Join())
			},
			func() {
				t.Log("Wait should work just as good as Join")
				g.Wait()
			},
		)()

		assert.Equal(t, 0, g.Len(), "it was expected that we waited for all the jobs")
	})
}

func TestJobGroup_asManual(t *testing.T) {
	t.Run("finished jobs are NOT garbage collected", func(t *testing.T) {
		var g = tasker.JobGroup[tasker.Manual]{}

		ctx := context.Background()

		done := make(chan struct{})

		n := rnd.Repeat(3, 7, func() {
			g.Background(ctx, func(ctx context.Context) error {
				<-done
				return nil
			})
		})

		assert.Eventually(t, time.Second, func(t testing.TB) {
			assert.Equal(t, n, g.Len())
		})

		close(done)

		assert.Equal(t, n, g.Len())

		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, g.Join())
		})

		assert.Equal(t, 0, g.Len())
	})
	t.Run("error is returned back", func(t *testing.T) {
		var g = tasker.JobGroup[tasker.Manual]{}

		ctx := context.Background()

		done := make(chan struct{})

		var errs []error
		rnd.Repeat(3, 7, func() {
			err := rnd.Error()
			errs = append(errs, err)
			g.Background(ctx, func(ctx context.Context) error {
				<-done
				return err
			})
		})

		close(done)

		gotErr := g.Join()
		assert.Error(t, gotErr)
		for _, err := range errs {
			assert.ErrorIs(t, gotErr, err)
		}

		assert.Equal(t, 0, g.Len(), "after Join, it was expected that we waited for all the ")
	})
}

func TestJobGroup_Go_wManual(t *testing.T) {
	var g = tasker.JobGroup[tasker.Manual]{}

	done := make(chan struct{})

	n := rnd.Repeat(3, 7, func() {
		g.Go(func(ctx context.Context) error {
			<-done
			return nil
		})
	})

	assert.Eventually(t, time.Second, func(t testing.TB) {
		assert.Equal(t, n, g.Len())
	})

	close(done)

	assert.Equal(t, n, g.Len())

	assert.Within(t, time.Second, func(ctx context.Context) {
		assert.NoError(t, g.Join())
	})

	assert.Equal(t, 0, g.Len())

	expErr := rnd.Error()

	g.Go(func(ctx context.Context) error {
		return expErr
	})

	assert.ErrorIs(t, g.Join(), expErr)
}

func TestJob(t *testing.T) {
	t.Run("nil task", func(t *testing.T) {
		var j tasker.Job
		err := j.Start(context.Background(), nil)
		assert.NoError(t, err)
	})

	t.Run("already alive", func(t *testing.T) {
		var j tasker.Job
		done := make(chan struct{})
		defer close(done)
		err := j.Start(context.Background(), func(ctx context.Context) error {
			<-done
			return nil
		})
		assert.NoError(t, err) // Start returns before task starts

		err = j.Start(context.Background(), func(ctx context.Context) error { return nil })
		assert.ErrorIs(t, err, tasker.ErrAlive) // Second start returns ErrAlive
	})

	t.Run("successful start", func(t *testing.T) {
		var j tasker.Job
		task := func(ctx context.Context) error { return nil }
		err := j.Start(context.Background(), task)
		assert.NoError(t, err)
		assert.NoError(t, j.Join())
	})

	t.Run("error during job execution", func(t *testing.T) {
		var j tasker.Job
		expErr := rnd.Error()
		task := func(ctx context.Context) error { return expErr }
		err := j.Start(context.Background(), task)
		assert.NoError(t, err)
		assert.ErrorIs(t, j.Join(), expErr)
	})

	t.Run("Stop cancels the context of the running task", func(t *testing.T) {
		var j tasker.Job

		expErr := rnd.Error()

		assert.NoError(t, j.Start(context.Background(), func(ctx context.Context) error {
			<-ctx.Done()
			return expErr
		}))

		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.ErrorIs(t, expErr, j.Stop())
		})
	})

	t.Run("Stop/Join doesn't returns back context cancellation", func(t *testing.T) {
		var j tasker.Job

		assert.NoError(t, j.Start(context.Background(), func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}))

		assert.Within(t, time.Second, func(ctx context.Context) {
			assert.NoError(t, j.Stop())
			assert.NoError(t, j.Join())
		})
	})
}
