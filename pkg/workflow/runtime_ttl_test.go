package workflow_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

func specRuntimeTTL(s *testcase.Spec, runtime testcase.Var[workflow.Runtime]) {
	var (
		ttl          = let.VarOf(s, 3*time.Minute)
		activityTime = let.VarOf(s, time.Minute)
		replayTime   = let.VarOf(s, time.Duration(0))
		result       = let.VarOf[error](s, nil)
		calls        = let.Var(s, func(t *testcase.T) []string { return nil })
		value        = let.String(s)
		process      = wftest.LetProcessID(s)
		definition   = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				wftest.Stub{StubExecute: func(context.Context, workflow.ProcessID) error {
					timecop.Travel(t, replayTime.Get(t))
					return nil
				}},
				workflow.ExecuteParticipant{ID: "first", Output: []workflow.VarName{"value"}},
				workflow.ExecuteParticipant{ID: "second", Input: []workflow.VarName{"value"}},
			}
		})
	)

	runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		rt := runtime.Super(t)
		rt.TTL = ttl.Get(t)
		rt.RetryStrategy = singleAttempt{}
		rt.Participants = workflow.Participants{
			"first": func(ctx context.Context) (string, error) {
				timecop.Travel(t, activityTime.Get(t))
				assert.NoError(t, ctx.Err(), "TTL must not cancel an in-flight activity")
				testcase.Append(t, calls, "first")
				return value.Get(t), result.Get(t)
			},
			"second": func(ctx context.Context, got string) error {
				timecop.Travel(t, time.Minute)
				assert.Equal(t, got, value.Get(t))
				testcase.Append(t, calls, "second")
				return nil
			},
		}
		return rt
	})

	s.Before(func(t *testcase.T) {

		assert.Must(t).NoError(runtime.Get(t).Bind(t.Context(), process.Get(t), definition.Get(t)))
	})

	s.Describe("#TTL", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return runtime.Get(t).Execute(t.Context(), process.Get(t))
		}
		isCompleted := func(t *testcase.T) bool {
			done, err := workflow.IsCompleted(t.Context(), runtime.Get(t).Events, process.Get(t))
			assert.NoError(t, err)
			return done
		}

		s.Test("finishes without suspending while within the budget", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.Equal(t, calls.Get(t), []string{"first", "second"})
			assert.True(t, isCompleted(t))
		})

		s.When("an activity reaches the TTL", func(s *testcase.Spec) {
			activityTime.Let(s, ttl.Get)

			s.Then("finishes and saves that activity before yielding, without starting the next", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), []string{"first"})
				assert.Equal[any](t, getVar(t, runtime.Get(t), process.Get(t), "value"), value.Get(t))
				assert.False(t, isCompleted(t))
			})

			s.Then("resumes with a fresh budget and reuses the saved result", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				timecop.Travel(t, time.Hour)
				assert.NoError(t, act(t))
				assert.Equal(t, calls.Get(t), []string{"first", "second"})
				assert.True(t, isCompleted(t))
			})
		})

		s.When("several activities together exhaust the budget", func(s *testcase.Spec) {
			ttl.LetValue(s, 2*time.Minute)

			s.Then("shares the budget across activities rather than resetting it for each one", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), []string{"first", "second"})
				assert.False(t, isCompleted(t))
				assert.NoError(t, act(t))
				assert.True(t, isCompleted(t))
			})
		})

		s.When("an activity runs beyond the TTL", func(s *testcase.Spec) {
			activityTime.LetValue(s, time.Hour)

			s.Then("waits for the activity to finish before yielding", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), []string{"first"})
			})
		})

		s.When("TTL is zero", func(s *testcase.Spec) {
			ttl.LetValue(s, 0)

			s.Then("runs to completion without a time budget", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.True(t, isCompleted(t))
			})
		})

		s.When("TTL is negative", func(s *testcase.Spec) {
			ttl.LetValue(s, -time.Minute)

			s.Then("runs to completion without a time budget", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.True(t, isCompleted(t))
			})
		})

		s.When("replaying the definition takes longer than TTL", func(s *testcase.Spec) {
			replayTime.LetValue(s, time.Hour)
			ttl.LetValue(s, time.Nanosecond)

			s.Then("makes new progress on each pass and completes even with an expired cache-only replay", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), []string{"first"})
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), []string{"first", "second"})
				assert.NoError(t, act(t))
				assert.True(t, isCompleted(t))
				assert.Equal(t, calls.Get(t), []string{"first", "second"})
			})
		})

		s.When("an activity fails after TTL expires", func(s *testcase.Spec) {
			activityTime.LetValue(s, time.Hour)
			result.Let(s, func(t *testcase.T) error { return t.Random.Error() })

			s.Then("returns the activity error rather than suspending", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), result.Get(t))
				assert.Equal(t, calls.Get(t), []string{"first"})
				assert.False(t, isCompleted(t))
			})
		})

		s.When("an activity signals Halt after TTL expires", func(s *testcase.Spec) {
			activityTime.LetValue(s, time.Hour)
			result.LetValue(s, workflow.Halt{})

			s.Then("honours the explicit signal rather than suspending", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Halt{})
				assert.Equal(t, calls.Get(t), []string{"first"})
			})
		})

		s.When("a condition reaches TTL while choosing a branch", func(s *testcase.Spec) {
			runtime.Let(s, func(t *testcase.T) workflow.Runtime {
				rt := runtime.Super(t)
				rt.Conditions = workflow.Conditions{
					"choose": func(context.Context) (bool, error) {
						timecop.Travel(t, ttl.Get(t))
						testcase.Append(t, calls, "choose")
						return true, nil
					},
				}
				return rt
			})
			definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.If{
					Cond: workflow.ExecuteCondition{ID: "choose"},
					Then: workflow.SetVar{Name: "chosen", Value: true},
				}
			})

			s.Then("saves the answer and resumes the chosen branch without evaluating again", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.False(t, isCompleted(t))
				assert.NoError(t, act(t))
				assert.Equal(t, calls.Get(t), []string{"choose"})
				assert.Equal[any](t, getVar(t, runtime.Get(t), process.Get(t), "chosen"), true)
				assert.True(t, isCompleted(t))
			})
		})

		s.When("an activity returns follow-up work that exceeds TTL", func(s *testcase.Spec) {
			runtime.Let(s, func(t *testcase.T) workflow.Runtime {
				rt := runtime.Super(t)
				rt.Participants.(workflow.Participants)["first"] = func(context.Context) error {
					testcase.Append(t, calls, "first")
					return workflow.Sequence{
						workflow.SetVar{Name: "value", Value: value.Get(t)},
						workflow.ExecuteParticipant{ID: "second", Input: []workflow.VarName{"value"}},
						workflow.ExecuteParticipant{ID: "second", Input: []workflow.VarName{"value"}},
					}
				}
				return rt
			})
			ttl.LetValue(s, time.Minute)
			definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.ExecuteParticipant{ID: "first"}
			})

			s.Then("finishes the follow-up before yielding so replay cannot skip unfinished work", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), []string{"first", "second", "second"})
				assert.NoError(t, act(t))
				assert.True(t, isCompleted(t))
				assert.Equal(t, calls.Get(t), []string{"first", "second", "second"})
			})
		})
	})

	s.Describe("#TTL scheduling", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return runtime.Get(t).Schedule(t.Context(), process.Get(t))
		}

		runtime.Let(s, func(t *testcase.T) workflow.Runtime {
			rt := runtime.Super(t)
			rt.TTL = time.Nanosecond
			rt.WaitTime = time.Nanosecond
			return rt
		})

		s.Test("automatically resumes TTL suspensions until all work completes exactly once", func(t *testcase.T) {
			rt, pid := runtime.Get(t), process.Get(t)
			assert.NoError(t, act(t))
			assert.Eventually(t, 5*time.Second, func(tb testing.TB) {
				done, err := workflow.IsCompleted(t.Context(), rt.Events, pid)
				assert.NoError(tb, err)
				assert.True(tb, done)
			})
			assert.Equal(t, calls.Get(t), []string{"first", "second"})
		})
	})
}
