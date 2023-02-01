package sysutil_test

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/sysutil"
	"github.com/adamluzsi/frameless/pkg/sysutil/internal"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/let"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func ExampleShutdownManager() {
	simpleJob := func(signal context.Context) error {
		<-signal.Done() // work until shutdown signal
		return signal.Err()
	}

	httpServerJob := func(signal context.Context) error {
		srv := http.Server{
			Addr: "localhost:8080",
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}),
		}
		go func() {
			if err := srv.ListenAndServe(); err != nil {
				log.Println("ERROR", err.Error())
			}
		}()
		<-signal.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}

	sm := sysutil.ShutdownManager{
		Jobs: []sysutil.Job{ // each Job will run on its own goroutine.
			simpleJob,
			httpServerJob,
		},
	}

	if err := sm.Run(context.Background()); err != nil {
		log.Println("ERROR", err.Error())
	}
}

var _ sysutil.Job = sysutil.ShutdownManager{}.Run

func TestShutdownManager(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		jobs    = testcase.LetValue[[]sysutil.Job](s, nil)
		signals = testcase.LetValue[[]os.Signal](s, nil)
	)
	subject := testcase.Let(s, func(t *testcase.T) sysutil.ShutdownManager {
		return sysutil.ShutdownManager{
			Jobs:    jobs.Get(t),
			Signals: signals.Get(t),
		}
	})

	s.Describe(".Run", func(s *testcase.Spec) {
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
		act := func(t *testcase.T) error {
			return subject.Get(t).Run(Context.Get(t))
		}

		s.When("no job is provided", func(s *testcase.Spec) {
			jobs.LetValue(s, nil)

			s.Then("it returns early", func(t *testcase.T) {
				t.Must.Within(time.Second, func(ctx context.Context) {
					Context.Set(t, ctx)

					t.Must.NoError(act(t))
				})
			})
		})

		s.When("signal is provided", func(s *testcase.Spec) {
			signals.Let(s, func(t *testcase.T) []os.Signal {
				return []os.Signal{
					t.Random.SliceElement([]os.Signal{
						syscall.SIGINT,
						syscall.SIGHUP,
						syscall.SIGTERM,
					}).(os.Signal),
				}
			})

			s.Then("it will use the signals to subscribe for notify", func(t *testcase.T) {
				var run bool
				StubSignalNotify(t, func(c chan<- os.Signal, sigs ...os.Signal) {
					run = true
					t.Must.ContainExactly(signals.Get(t), sigs)
				})

				t.Must.Within(time.Second, func(context.Context) {
					t.Must.NoError(act(t))
				}) // no job should mean no block

				t.Must.True(run)
			})
		})

		s.When("jobs are provided", func(s *testcase.Spec) {
			othJob := testcase.Let(s, func(t *testcase.T) sysutil.Job {
				return func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				}
			})

			jobDone := testcase.LetValue[bool](s, false)
			jobs.Let(s, func(t *testcase.T) []sysutil.Job {
				return []sysutil.Job{
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
				othJob.Let(s, func(t *testcase.T) sysutil.Job {
					return func(ctx context.Context) error {
						return nil
					}
				})

				s.Then("it will block and doesn't affect the other jobs", func(t *testcase.T) {
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

				othJob.Let(s, func(t *testcase.T) sysutil.Job {
					return func(ctx context.Context) error {
						return expectedErr.Get(t)
					}
				})

				s.Then("it will not block but signal shutdown and return all doesn't affect the other jobs", func(t *testcase.T) {
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
	})
}

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
