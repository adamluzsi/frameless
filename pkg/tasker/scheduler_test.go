package tasker_test

import (
	"context"
	"log"
	"runtime"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

func ExampleScheduler_WithSchedule() {
	scheduler := tasker.Scheduler{}

	task := scheduler.WithSchedule("db maintenance", tasker.Monthly{Day: 1}, func(ctx context.Context) error {
		return nil
	})

	if err := task(context.Background()); err != nil {
		log.Println("ERROR", err.Error())
	}
}

const blockCheckWaitTime = 42 * time.Millisecond

func TestScheduler(t *testing.T) {
	s := testcase.NewSpec(t, testcase.Flaky(3))
	s.HasSideEffect()

	var (
		repository = testcase.Let(s, func(t *testcase.T) tasker.ScheduleStateRepository {
			return memory.NewTaskerSchedulerStateRepository()
		})
		locks = testcase.Let(s, func(t *testcase.T) tasker.SchedulerLocks {
			return memory.NewTaskerSchedulerLocks()
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) tasker.Scheduler {
		return tasker.Scheduler{
			Locks:  locks.Get(t),
			States: repository.Get(t),
		}
	})

	s.Describe(".WithSchedule", func(s *testcase.Spec) {
		var (
			id       = let.As[tasker.ScheduleID](let.String(s))
			interval = let.As[time.Duration](let.IntB(s, int(time.Hour), 24*int(time.Hour)))

			ran  = testcase.LetValue[int](s, 0)
			task = testcase.Let(s, func(t *testcase.T) tasker.Task {
				return func(ctx context.Context) error {
					ran.Set(t, ran.Get(t)+1)
					return nil
				}
			})
		)
		act := func(t *testcase.T) tasker.Task {
			return subject.Get(t).WithSchedule(id.Get(t), tasker.Every(interval.Get(t)), task.Get(t))
		}

		var Context = let.Context(s)

		s.Then("the resulting job will be a blocking job", func(t *testcase.T) {
			t.Must.NotWithin(blockCheckWaitTime, func(ctx context.Context) {
				gotErr := act(t)(ctx)
				t.Must.AnyOf(func(a *assert.A) {
					a.Test(func(t assert.It) { t.Must.NoError(gotErr) })
					a.Test(func(t assert.It) { t.Must.ErrorIs(ctx.Err(), gotErr) })
				})
			})
		})

		s.Then("the passed Job func will be executed based on the interval time", func(t *testcase.T) {
			go act(t)(Context.Get(t))

			for i := 0; i < 7; i++ {
				t.Must.Within(time.Second, func(ctx context.Context) {
					t.Eventually(func(it *testcase.T) {
						it.Must.Equal(i+1, ran.Get(t))
					})
				})

				timecop.Travel(t, interval.Get(t)+time.Second)
			}
		})

		s.Then("the passed Job func will not run faster than the expected interval", func(t *testcase.T) {
			go act(t)(Context.Get(t))
			t.Eventually(func(it *testcase.T) {
				it.Must.Equal(1, ran.Get(t))
			})
			timecop.Travel(t, interval.Get(t)/2)
			t.Must.True(time.Millisecond < interval.Get(t),
				"to make here the sleep safe, the interval must be larger than a millisecond")
			time.Sleep(time.Millisecond)
			t.Must.Equal(1, ran.Get(t))
		})

		s.Then("concurrently competing tasker guaranteed to not do the job twice", func(t *testcase.T) {
			t.Random.Repeat(3, 7, func() {
				go act(t)(Context.Get(t))
			})

			for i := 0; i < 7; i++ {
				t.Must.Within(time.Second, func(ctx context.Context) {
					t.Eventually(func(it *testcase.T) {
						it.Must.Equal(i+1, ran.Get(t))
					})
				})

				timecop.Travel(t, interval.Get(t)+time.Second)
			}
		})

		s.When("error occurs in the job", func(s *testcase.Spec) {
			expErr := let.Error(s)
			task.Let(s, func(t *testcase.T) tasker.Task {
				return func(ctx context.Context) error {
					return expErr.Get(t)
				}
			})

			s.Then("error is returned", func(t *testcase.T) {
				t.Must.Within(time.Second, func(ctx context.Context) {
					t.Must.ErrorIs(expErr.Get(t), act(t)(ctx))
				})
			})
		})

		s.When("error occurs eventually in the job", func(s *testcase.Spec) {
			interval.LetValue(s, 0)
			expErr := let.Error(s)
			task.Let(s, func(t *testcase.T) tasker.Task {
				var ok bool
				return func(ctx context.Context) error {
					if !ok {
						ok = true
						return nil
					}
					return expErr.Get(t)
				}
			})

			s.Then("error is returned", func(t *testcase.T) {
				t.Must.Within(time.Second, func(ctx context.Context) {
					t.Must.ErrorIs(expErr.Get(t), act(t)(ctx))
				})
			})
		})
	})
}

func TestWithNoOverlap(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		lock = testcase.Let(s, func(t *testcase.T) guard.NonBlockingLocker {
			return memory.NewLocker()
		})
		ran  = testcase.LetValue[int](s, 0)
		task = testcase.Let(s, func(t *testcase.T) tasker.Task {
			return func(ctx context.Context) error {
				ran.Set(t, ran.Get(t)+1)
				return nil
			}
		})
	)
	act := func(t *testcase.T) tasker.Task {
		return tasker.WithNoOverlap(lock.Get(t), task.Get(t))
	}

	s.Test("The task can execute as many times we want", func(t *testcase.T) {
		tsk := act(t)

		t.Random.Repeat(3, 5, func() {
			current := ran.Get(t)
			assert.NoError(t, tsk.Run(context.Background()))
			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, ran.Get(t), current+1)
			})
		})
	})

	s.When("a task with our ID is already running", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			bgTask := tasker.WithNoOverlap(lock.Get(t), func(ctx context.Context) error {
				<-t.Done() // run until the end of the test
				return ctx.Err()
			})
			go bgTask.Run(context.Background())
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		})

		s.Then("new task execution will not execute but return early instead", func(t *testcase.T) {
			assert.Within(t, time.Second, func(ctx context.Context) {
				assert.NoError(t, act(t).Run(context.Background()))
			})

			assert.Equal(t, 0, ran.Get(t))
		})
	})

	// s.When(".Locks field is not supplied", func(s *testcase.Spec) {
	// 	lock.LetValue(s, nil)

	// 	s.Then("the global lock factory is used and at least within the same process, the no-overlapping is ensured", func(t *testcase.T) {
	// 		ctx := context.Background()
	// 		tsk := act(t)

	// 		n := t.Random.Repeat(3, 5, func() {
	// 			current := ran.Get(t)
	// 			assert.NoError(t, tsk.Run(ctx))
	// 			t.Eventually(func(t *testcase.T) {
	// 				assert.Equal(t, ran.Get(t), current+1)
	// 			})
	// 		})

	// 		go subject.Get(t).WithNoOverlap(id.Get(t), func(ctx context.Context) error {
	// 			<-t.Done() // run until the end of the test
	// 			return nil
	// 		}).Run(ctx)

	// 		runtime.Gosched()
	// 		time.Sleep(time.Millisecond)

	// 		assert.Within(t, time.Second, func(ctx context.Context) {
	// 			assert.NoError(t, act(t).Run(ctx))
	// 		})

	// 		assert.Equal(t, n, ran.Get(t))
	// 	})
	// })
}

// func TestScheduler_WithNoOverlap_zeroSchedulerUseInMemGlobalLock(t *testing.T) {
// 	scheduler := tasker.Scheduler{}
// 	const id = "foo"

// 	var count int32
// 	job := scheduler.WithNoOverlap(id, func(ctx context.Context) error {
// 		atomic.AddInt32(&count, 1)
// 		return nil
// 	})

// 	total := rnd.Repeat(3, 5, func() {
// 		current := atomic.LoadInt32(&count)
// 		assert.NoError(t, job.Run(context.Background()))

// 		assert.Eventually(t, time.Second, func(t assert.It) {
// 			assert.Equal(t, atomic.LoadInt32(&count), current+1)
// 		})
// 	})

// 	runtime.Gosched()
// 	time.Sleep(time.Millisecond)

// 	done := make(chan struct{})
// 	bgtsk := scheduler.WithNoOverlap(id, func(ctx context.Context) error {
// 		<-done // run until the end of the test
// 		return ctx.Err()
// 	})
// 	go bgtsk.Run(context.Background())
// 	runtime.Gosched()
// 	time.Sleep(time.Millisecond)

// 	assert.Within(t, time.Second, func(ctx context.Context) {
// 		assert.NoError(t, job.Run(context.Background()))
// 	})

// 	assert.Equal(t, int32(total), atomic.LoadInt32(&count))
// }
