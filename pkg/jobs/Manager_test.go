package jobs_test

import (
	"context"
	jobspkg "github.com/adamluzsi/frameless/pkg/jobs"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
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

var _ runnable = jobspkg.Manager{}

func ExampleManager() {
	simpleJob := func(signal context.Context) error {
		<-signal.Done() // work until shutdown signal
		return signal.Err()
	}

	srv := http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}

	httpServerJob := jobspkg.WithShutdown(srv.ListenAndServe, srv.Shutdown)

	sm := jobspkg.Manager{
		Jobs: []jobspkg.Job{ // each Job will run on its own goroutine.
			simpleJob,
			httpServerJob,
		},
	}

	if err := sm.Run(context.Background()); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func ExampleRun() {
	simpleJob := func(signal context.Context) error {
		<-signal.Done() // work until shutdown signal
		return signal.Err()
	}

	srv := http.Server{
		Addr: "localhost:8080",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
	}

	httpServerJob := jobspkg.WithShutdown(srv.ListenAndServe, srv.Shutdown)

	if err := jobspkg.Run(context.Background(), simpleJob, httpServerJob); err != nil {
		log.Println("ERROR", err.Error())
	}
}

func TestRun_smoke(t *testing.T) {
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
		gotErr = jobspkg.Run(baseCTX, func(ctx context.Context) error {
			assert.Equal(t, value, ctx.Value(key).(string))
			<-ctx.Done()
			return expErr
		})
	}, "expected to block")

	cancel()
	wg.Wait()
	assert.Equal(t, expErr, gotErr)
}

var _ jobspkg.Job = jobspkg.Manager{}.Run

func TestManager(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		jobs    = testcase.LetValue[[]jobspkg.Job](s, nil)
		signals = testcase.LetValue[[]os.Signal](s, nil)
	)
	subject := testcase.Let(s, func(t *testcase.T) jobspkg.Manager {
		return jobspkg.Manager{
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
			othJob := testcase.Let(s, func(t *testcase.T) jobspkg.Job {
				return func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				}
			})

			jobDone := testcase.LetValue[bool](s, false)
			jobs.Let(s, func(t *testcase.T) []jobspkg.Job {
				return []jobspkg.Job{
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
				othJob.Let(s, func(t *testcase.T) jobspkg.Job {
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

				othJob.Let(s, func(t *testcase.T) jobspkg.Job {
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
