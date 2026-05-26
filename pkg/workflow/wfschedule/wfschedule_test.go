package wfschedule_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftesting"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"

	. "go.llib.dev/frameless/pkg/workflow/wftesting"
)

const timeout = time.Second / 2

func TestScheduler(t *testing.T) {
	s := testcase.NewSpec(t)
	c := LetC(s)

	var (
		pid = LetParticipantID(s)

		callCount = let.VarOf(s, 0)
		_         = LetParticipant(s, c, pid, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				callCount.Set(t, callCount.Get(t)+1)
				return nil
			}
		})
	)

	subject := c.Scheduler.Bind(s)

	s.Describe("#Schedule and #Run", func(s *testcase.Spec) {
		var (
			Context   = let.Context(s)
			process   = LetProcess(s)
			startTime = let.Var(s, func(t *testcase.T) time.Time {
				return time.Time{}
			})
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Schedule(Context.Get(t), process.Get(t), startTime.Get(t))
		})

		s.When("process's definition succeeds without an issue", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("then upon scheduling, eventually Schedule#Run will process the process task", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, callCount.Get(t), 1)
				})
			})

			s.And("the start time is somewhere in the future", func(s *testcase.Spec) {
				startTime.Let(s, func(t *testcase.T) time.Time {
					return clock.Now().Add(time.Hour)
				})

				s.Before(func(t *testcase.T) {
					timecop.Travel(t, time.Nanosecond)
				})

				s.Then("execution won't occur until the start time reached", func(t *testcase.T) {
					assert.NoError(t, act(t))

					w := assert.NotWithin(t, timeout, func(ctx context.Context) {
						for callCount.Get(t) == 1 {
							select {
							case <-t.Done():
								return
							default: // OK
							}
						}
					})

					t.Log("but after enough time was waited")
					timecop.Travel(t, time.Hour+time.Minute)

					assert.Within(t, timeout, func(ctx context.Context) {
						w.Wait()
					})
				})
			})
		})

		s.When("process has no ID", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.ID = workflow.ProcessID{} // zero value
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("a new ID is generated and assigned to the process", func(t *testcase.T) {
				assert.NoError(t, act(t))

				t.Eventually(func(t *testcase.T) {
					assert.NotEmpty(t, process.Get(t).ID)
				})
			})
		})

		s.When("process is scheduled multiple times", func(s *testcase.Spec) {
			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("scheduling remains idempotent and the participant is called only once", func(t *testcase.T) {
				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))
				})

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, callCount.Get(t), 1)
				})
			})
		})

		s.When("process definition suspends", func(s *testcase.Spec) {
			shouldSuspend := let.VarOf(s, true)

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.Sequence{
					workflow.ExecuteParticipant{ID: pid.Get(t)},
					workflow.Suspend{While: wftesting.Stub{StubEvaluate: func(ctx context.Context, p *workflow.Process) (bool, error) {
						return shouldSuspend.Get(t), nil
					}}},
				}
				return p
			})

			s.Then("the participant is executed but process remains incomplete until suspend allows it to pass", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.NotWithin(t, timeout, func(ctx context.Context) {
					for {
						if callCount.Get(t) == 1 {
							break
						}
						select {
						case <-t.Done():
							return
						default: // OK
						}
					}
				})

				shouldSuspend.Set(t, false)

				t.Eventually(func(t *testcase.T) {
					assert.Equal(t, callCount.Get(t), 1)
				})
			})
		})

		s.When("context is cancelled during scheduling", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // cancel immediately
				return ctx
			})

			process.Let(s, func(t *testcase.T) *workflow.Process {
				p := process.Super(t)
				p.Definition = workflow.ExecuteParticipant{
					ID: pid.Get(t),
				}
				return p
			})

			s.Then("scheduling fails with context cancellation error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), context.Canceled)
			})
		})
	})
}
