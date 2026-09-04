package workflow_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/workflow"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/let"
)

func Test_e2e(tt *testing.T) {
	s := testcase.NewSpec(tt)

	s.Test("smoke", func(t *testcase.T) {
		var (
			fooOut = t.Random.String()
			barOut = t.Random.Int()
		)

		participants := workflow.Participants{
			"foo": func(ctx context.Context) (string, error) {
				return fooOut, nil
			},
			"bar": func(ctx context.Context, in string) (int, error) {
				assert.Equal(t, in, fooOut)
				return barOut, nil
			},
			"baz": func(ctx context.Context, s string, n int) error {
				assert.Equal(t, fooOut, s)
				assert.Equal(t, barOut, n)
				return nil
			},
		}

		var pdef workflow.Definition = &workflow.Sequence{
			&workflow.ExecuteParticipant{
				ID:     "foo",
				Output: []workflow.VarName{"foo-val"},
			},
			&workflow.ExecuteParticipant{
				ID:     "bar",
				Input:  []workflow.VarName{"foo-val"},
				Output: []workflow.VarName{"bar-val"},
			},
			&workflow.ExecuteParticipant{
				ID:    "baz",
				Input: []workflow.VarName{"foo-val", "bar-val"},
			},
		}

		r := workflow.Runtime{
			Participants: participants,
			Events:       &memory.WorkflowEventRepository{},
			Locks:        &memory.WorkflowProcessLocks{},
		}

		p := mustProcessID(t)

		assert.NoError(t, pdef.Execute(r.Context(t.Context()), p))
		assert.Equal[any](t, getVar(t, r, p, "foo-val"), fooOut)
		assert.Equal[any](t, getVar(t, r, p, "bar-val"), barOut)

	})

	s.Test("definition idempotency", func(t *testcase.T) {
		var (
			fooOut = t.Random.String()
			barOut = t.Random.Int()

			expectedFlakyErr = t.Random.Error()
			failOnce         sync.Once
		)

		var ranCount = map[string]int{}
		var inc = func(n string) {
			ranCount[n] = ranCount[n] + 1
		}

		participants := workflow.Participants{
			"foo": func(ctx context.Context) (string, error) {
				inc("foo")
				return fooOut, nil
			},
			"bar": func(ctx context.Context, in string) (int, error) {
				inc("bar")
				assert.Equal(t, in, fooOut)
				return barOut, nil
			},
			"baz": func(ctx context.Context, s string, n int) error {
				inc("baz")
				assert.Equal(t, fooOut, s)
				assert.Equal(t, barOut, n)
				return nil
			},
			"flaky": func(ctx context.Context) (err error) {
				inc("flaky")
				failOnce.Do(func() {
					err = expectedFlakyErr
				})
				return err
			},
		}

		var def workflow.Definition = &workflow.Sequence{
			workflow.ExecuteParticipant{
				ID:     "foo",
				Output: []workflow.VarName{"foo-val"},
			},
			workflow.ExecuteParticipant{
				ID:     "bar",
				Input:  []workflow.VarName{"foo-val"},
				Output: []workflow.VarName{"bar-val"},
			},
			workflow.ExecuteParticipant{
				ID:    "baz",
				Input: []workflow.VarName{"foo-val", "bar-val"},
			},
			workflow.ExecuteParticipant{
				ID: "flaky",
				//TODO: retry integration maybe?
			},
		}

		r := workflow.Runtime{
			Participants: participants,
			Events:       &memory.WorkflowEventRepository{},
			Locks:        &memory.WorkflowProcessLocks{},
		}

		p := mustProcessID(t)
		assert.ErrorIs(t, expectedFlakyErr, def.Execute(r.Context(t.Context()), p))
		assert.NotEmpty(t, mustHistory(t, r, p))

		assert.NoError(t, def.Execute(r.Context(t.Context()), p))
		assert.Equal[any](t, getVar(t, r, p, "foo-val"), fooOut)
		assert.Equal[any](t, getVar(t, r, p, "bar-val"), barOut)
		assert.Equal(t, ranCount["foo"], 1)
		assert.Equal(t, ranCount["bar"], 1)
		assert.Equal(t, ranCount["baz"], 1)
		assert.Equal(t, ranCount["flaky"], 2)
	})

	s.Test("scheduling", func(t *testcase.T) {
		var (
			fooOut = t.Random.String()
			barOut = t.Random.Int()

			expectedFlakyErr = t.Random.Error()
			failOnce         sync.Once
		)

		var ranCount = map[string]int{}
		var inc = func(n string) {
			ranCount[n] = ranCount[n] + 1
		}

		participants := workflow.Participants{
			"foo": func(ctx context.Context) (string, error) {
				inc("foo")
				return fooOut, nil
			},
			"bar": func(ctx context.Context, in string) (int, error) {
				inc("bar")
				assert.Equal(t, in, fooOut)
				return barOut, nil
			},
			"baz": func(ctx context.Context, s string, n int) error {
				inc("baz")
				assert.Equal(t, fooOut, s)
				assert.Equal(t, barOut, n)
				return nil
			},
			"flaky": func(ctx context.Context) (err error) {
				inc("flaky")
				failOnce.Do(func() {
					err = expectedFlakyErr
				})
				return err
			},
		}

		var def workflow.Definition = &workflow.Sequence{
			workflow.ExecuteParticipant{
				ID:     "foo",
				Output: []workflow.VarName{"foo-val"},
			},
			workflow.ExecuteParticipant{
				ID:     "bar",
				Input:  []workflow.VarName{"foo-val"},
				Output: []workflow.VarName{"bar-val"},
			},
			workflow.ExecuteParticipant{
				ID:    "baz",
				Input: []workflow.VarName{"foo-val", "bar-val"},
			},
			workflow.ExecuteParticipant{
				ID: "flaky",
				//TODO: retry integration maybe?
			},
		}

		r := workflow.Runtime{
			Participants: participants,
			Events:       &memory.WorkflowEventRepository{},
			Locks:        &memory.WorkflowProcessLocks{},
		}

		p := mustProcessID(t)
		assert.ErrorIs(t, expectedFlakyErr, def.Execute(r.Context(t.Context()), p))
		assert.NotEmpty(t, mustHistory(t, r, p))

		assert.NoError(t, def.Execute(r.Context(t.Context()), p))
		assert.Equal[any](t, getVar(t, r, p, "foo-val"), fooOut)
		assert.Equal[any](t, getVar(t, r, p, "bar-val"), barOut)
		assert.Equal(t, ranCount["foo"], 1)
		assert.Equal(t, ranCount["bar"], 1)
		assert.Equal(t, ranCount["baz"], 1)
		assert.Equal(t, ranCount["flaky"], 2)
	})
}

func TestContextWithParticipants(t *testing.T) {
	rt := workflow.Runtime{Events: &memory.WorkflowEventRepository{}}

	execFoo := workflow.ExecuteParticipant{ID: "foo"}
	execBar := workflow.ExecuteParticipant{ID: "bar"}

	procID, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	ctx0 := rt.Context(context.Background())
	assert.Error(t, execFoo.Execute(ctx0, procID))
	assert.Error(t, execBar.Execute(ctx0, procID))

	ctx1 := workflow.ContextWithParticipants(ctx0, workflow.Participants{"foo": func(ctx context.Context) error { return nil }})
	assert.Error(t, execFoo.Execute(ctx0, procID))
	assert.NoError(t, execFoo.Execute(ctx1, procID))
	assert.Error(t, execBar.Execute(ctx1, procID))

	ctx2 := workflow.ContextWithParticipants(ctx1, workflow.Participants{"bar": func(ctx context.Context) error { return nil }})
	assert.NoError(t, execFoo.Execute(ctx1, procID))
	assert.NoError(t, execFoo.Execute(ctx2, procID))
	assert.Error(t, execBar.Execute(ctx1, procID))
	assert.NoError(t, execBar.Execute(ctx2, procID))
}

func Test_pauseAndContinue(t *testing.T) {
	s := testcase.NewSpec(t)

	var counter = let.Var(s, func(t *testcase.T) map[string]int {
		return map[string]int{}
	})

	var inc = func(t *testcase.T, name string) {
		counter.Get(t)[name] = counter.Get(t)[name] + 1
	}

	var foo = let.Var(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			inc(t, "foo")
			return ctx.Err()
		}
	})
	var bar = let.Var(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			inc(t, "bar")
			return ctx.Err()
		}
	})

	var baz = let.Var(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			inc(t, "baz")
			return ctx.Err()
		}
	})

	rt := let.Var(s, func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Participants: workflow.Participants{
				"foo": foo.Get(t),
				"bar": bar.Get(t),
				"baz": baz.Get(t),
			},
			Events: &memory.WorkflowEventRepository{},
			Locks:  &memory.WorkflowProcessLocks{},
		}
	})

	def := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.ExecuteParticipant{ID: "foo"},
			workflow.ExecuteParticipant{ID: "bar"},
			workflow.ExecuteParticipant{ID: "baz"},
		}
	})

	s.Test("smoke", func(t *testcase.T) {
		pid := mustProcessID(t)
		// Process is stateless — bind the definition via the event history.
		rtCtx := rt.Get(t).Context(t.Context())
		var ev workflow.Event = workflow.EventUseDefinition{
			EventID:    mustEventID(t),
			ProcessID:  pid,
			Timestamp:  clock.Now(),
			Definition: def.Get(t),
		}
		assert.NoError(t, rt.Get(t).Events.Create(rtCtx, &ev))

		assert.NoError(t, rt.Get(t).Execute(t.Context(), pid))
		assert.Equal(t, counter.Get(t)["foo"], 1)
		assert.Equal(t, counter.Get(t)["bar"], 1)
		assert.Equal(t, counter.Get(t)["baz"], 1)
	})

	s.When("definition execution is interrupted midterm", func(s *testcase.Spec) {
		phaser := let.Phaser(s)

		bar.Let(s, func(t *testcase.T) func(ctx context.Context) error {
			fn := bar.Super(t)
			return func(ctx context.Context) error {
				if err := fn(ctx); err != nil {
					return err
				}
				phaser.Get(t).Wait()
				return ctx.Err()
			}
		})

		s.Then("workflow process can be recovered from a context cancellation", func(t *testcase.T) {
			ctx, cancel := context.WithCancel(t.Context())

			pid := mustProcessID(t)
			rtCtx := rt.Get(t).Context(t.Context())

			var ev workflow.Event = workflow.EventUseDefinition{
				EventID:    mustEventID(t),
				ProcessID:  pid,
				Timestamp:  clock.Now(),
				Definition: def.Get(t),
			}
			assert.NoError(t, rt.Get(t).Events.Create(rtCtx, &ev))

			var p = pid
			var gotErr error

			w := assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
				gotErr = rt.Get(t).Execute(ctx, p)
			})

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, 1, phaser.Get(t).Len())
			})

			cancel()
			phaser.Get(t).Finish()

			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				w.Wait()
			})

			assert.ErrorIs(t, ctx.Err(), gotErr)
			assert.Equal(t, counter.Get(t)["foo"], 1)
			assert.Equal(t, counter.Get(t)["bar"], 1)
			assert.Equal(t, counter.Get(t)["baz"], 0)

			t.Log("and then re-execution should be possible, and continuing from where it was left")
			t.Log("when the same process entity is used")
			assert.NoError(t, rt.Get(t).Execute(t.Context(), p))

			assert.Equal(t, counter.Get(t)["foo"], 1)
			assert.Equal(t, counter.Get(t)["bar"], 2, "expected that the failing bar is re-run")
			assert.Equal(t, counter.Get(t)["baz"], 1)
		})
	})
}

func withPath(ctx context.Context, path workflow.Path) context.Context {
	for _, name := range path {
		ctx = workflow.WithName(ctx, name)
	}
	return ctx
}
