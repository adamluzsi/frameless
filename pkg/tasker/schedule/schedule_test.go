package schedule_test

import (
	"context"
	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/tasker/schedule"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
	"log"
	"testing"
	"time"
)

func ExampleScheduler_WithSchedule() {
	scheduler := schedule.Scheduler{
		Repository: nil, // &postgresql.TaskerScheduleRepository{CM: cm},
	}

	task := scheduler.WithSchedule("db maintenance", schedule.Monthly{Day: 1}, func(ctx context.Context) error {
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
		repository = testcase.Let(s, func(t *testcase.T) schedule.Repository {
			return &memory.TaskerScheduleRepository{}
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) schedule.Scheduler {
		return schedule.Scheduler{
			Repository: repository.Get(t),
		}
	})

	s.Describe(".WithSchedule", func(s *testcase.Spec) {
		var (
			jobID    = let.As[schedule.StateID](let.String(s))
			interval = let.As[time.Duration](let.IntB(s, int(time.Hour), 24*int(time.Hour)))

			ran = testcase.LetValue[int](s, 0)
			job = testcase.Let(s, func(t *testcase.T) tasker.Task {
				return func(ctx context.Context) error {
					ran.Set(t, ran.Get(t)+1)
					return nil
				}
			})
		)
		act := func(t *testcase.T) tasker.Task {
			return subject.Get(t).WithSchedule(jobID.Get(t), schedule.Interval(interval.Get(t)), job.Get(t))
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
					t.Eventually(func(it assert.It) {
						it.Must.Equal(i+1, ran.Get(t))
					})
				})

				timecop.Travel(t, interval.Get(t)+time.Second)
			}
		})

		s.Then("the passed Job func will not run faster than the expected interval", func(t *testcase.T) {
			go act(t)(Context.Get(t))
			t.Eventually(func(it assert.It) {
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
					t.Eventually(func(it assert.It) {
						it.Must.Equal(i+1, ran.Get(t))
					})
				})

				timecop.Travel(t, interval.Get(t)+time.Second)
			}
		})

		s.When("error occurs in the job", func(s *testcase.Spec) {
			expErr := let.Error(s)
			job.Let(s, func(t *testcase.T) tasker.Task {
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
			job.Let(s, func(t *testcase.T) tasker.Task {
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
