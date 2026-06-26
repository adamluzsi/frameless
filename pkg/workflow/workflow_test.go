package workflow_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dscontract"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func Example() {
	rt := workflow.Runtime{
		Participants: workflow.Participants{
			"foo": func(ctx context.Context) (int, error) {
				return 42, nil
			},
			"bar": func(ctx context.Context) (int, error) {
				return 24, nil
			},
			"baz": func(ctx context.Context) error {
				return nil
			},
			"qux": func(ctx context.Context) error {
				return nil
			},
		},
	}

	userDefinedWorkflowDefinition := &workflow.Sequence{
		workflow.ExecuteParticipant{ID: "foo",
			Output: []workflow.VariableKey{"foo"}},
		workflow.ExecuteParticipant{ID: "bar",
			Output: []workflow.VariableKey{"bar"}},
		workflow.If{
			Cond: wftemplate.Condition(".foo <= .bar"),   // (42 < 24) == false
			Then: workflow.ExecuteParticipant{ID: "baz"}, //
			Else: workflow.ExecuteParticipant{ID: "qux"}, // will run
		},
	}

	_ = rt.Execute(context.Background(), &workflow.Process{Definition: userDefinedWorkflowDefinition})
}

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
				Output: []workflow.VariableKey{"foo-val"},
			},
			&workflow.ExecuteParticipant{
				ID:     "bar",
				Input:  []workflow.VariableKey{"foo-val"},
				Output: []workflow.VariableKey{"bar-val"},
			},
			&workflow.ExecuteParticipant{
				ID:    "baz",
				Input: []workflow.VariableKey{"foo-val", "bar-val"},
			},
		}

		r := workflow.Runtime{
			Participants: participants,
		}

		var p workflow.Process

		assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
		assert.Equal[any](t, p.Var().Get("foo-val"), fooOut)
		assert.Equal[any](t, p.Var().Get("bar-val"), barOut)

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
				Output: []workflow.VariableKey{"foo-val"},
			},
			workflow.ExecuteParticipant{
				ID:     "bar",
				Input:  []workflow.VariableKey{"foo-val"},
				Output: []workflow.VariableKey{"bar-val"},
			},
			workflow.ExecuteParticipant{
				ID:    "baz",
				Input: []workflow.VariableKey{"foo-val", "bar-val"},
			},
			workflow.ExecuteParticipant{
				ID: "flaky",
				//TODO: retry integration maybe?
			},
		}

		r := workflow.Runtime{
			Participants: participants,
		}

		var p workflow.Process
		assert.ErrorIs(t, expectedFlakyErr, def.Execute(r.Context(t.Context()), &p))
		assert.NotEmpty(t, mustHistory(t, &p))

		assert.NoError(t, def.Execute(r.Context(t.Context()), &p))
		assert.Equal[any](t, p.Var().Get("foo-val"), fooOut)
		assert.Equal[any](t, p.Var().Get("bar-val"), barOut)
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
				Output: []workflow.VariableKey{"foo-val"},
			},
			workflow.ExecuteParticipant{
				ID:     "bar",
				Input:  []workflow.VariableKey{"foo-val"},
				Output: []workflow.VariableKey{"bar-val"},
			},
			workflow.ExecuteParticipant{
				ID:    "baz",
				Input: []workflow.VariableKey{"foo-val", "bar-val"},
			},
			workflow.ExecuteParticipant{
				ID: "flaky",
				//TODO: retry integration maybe?
			},
		}

		r := workflow.Runtime{
			Participants: participants,
		}

		_ = workflow.Scheduler{}

		var p workflow.Process
		assert.ErrorIs(t, expectedFlakyErr, def.Execute(r.Context(t.Context()), &p))
		assert.NotEmpty(t, mustHistory(t, &p))

		assert.NoError(t, def.Execute(r.Context(t.Context()), &p))
		assert.Equal[any](t, p.Var().Get("foo-val"), fooOut)
		assert.Equal[any](t, p.Var().Get("bar-val"), barOut)
		assert.Equal(t, ranCount["foo"], 1)
		assert.Equal(t, ranCount["bar"], 1)
		assert.Equal(t, ranCount["baz"], 1)
		assert.Equal(t, ranCount["flaky"], 2)
	})
}

func TestProcess(t *testing.T) {
	s := testcase.NewSpec(t)

	process := let.Var(s, func(t *testcase.T) *workflow.Process {
		return &workflow.Process{}
	})

	s.Describe("Var", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) workflow.Vars {
			return process.Get(t).Var()
		})

		s.Context("implements ds.Map",
			dscontract.Map(func(tb testing.TB) ds.Map[workflow.VariableKey, any] {
				return act(tb.(*testcase.T))
			}).Spec)

		s.When("value is assigned with #Set", func(s *testcase.Spec) {
			key := let.As[workflow.VariableKey](let.String(s))
			value := let.Int(s)

			s.Before(func(t *testcase.T) {
				act(t).Set(key.Get(t), value.Get(t))
			})

			s.Then("the assigned value can be retrieved by its key with #Get", func(t *testcase.T) {
				assert.Equal[any](t, value.Get(t), act(t).Get(key.Get(t)))
			})

			s.Then("Process events used as a backing storage for recording variable states", func(t *testcase.T) {
				assert.NotEmpty(t, mustHistory(t, process.Get(t)))
			})
		})

		_ = act
	})
}

func TestContextWithParticipants(t *testing.T) {
	execFoo := workflow.ExecuteParticipant{ID: "foo"}
	execBar := workflow.ExecuteParticipant{ID: "bar"}

	ctx0 := workflow.WithExecutionIndex(context.Background())
	assert.Error(t, execFoo.Execute(ctx0, &workflow.Process{}))
	assert.Error(t, execBar.Execute(ctx0, &workflow.Process{}))

	ctx1 := workflow.ContextWithParticipants(ctx0, workflow.Participants{"foo": func(ctx context.Context) error { return nil }})
	assert.Error(t, execFoo.Execute(ctx0, &workflow.Process{}))
	assert.NoError(t, execFoo.Execute(ctx1, &workflow.Process{}))
	assert.Error(t, execBar.Execute(ctx1, &workflow.Process{}))

	ctx2 := workflow.ContextWithParticipants(ctx1, workflow.Participants{"bar": func(ctx context.Context) error { return nil }})
	assert.NoError(t, execFoo.Execute(ctx1, &workflow.Process{}))
	assert.NoError(t, execFoo.Execute(ctx2, &workflow.Process{}))
	assert.Error(t, execBar.Execute(ctx1, &workflow.Process{}))
	assert.NoError(t, execBar.Execute(ctx2, &workflow.Process{}))
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
		assert.NoError(t, rt.Get(t).Execute(t.Context(), &workflow.Process{Definition: def.Get(t)}))
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

			var p = workflow.Process{
				Definition: def.Get(t),
			}
			var gotErr error

			w := assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
				gotErr = rt.Get(t).Execute(ctx, &p)
			})

			t.Eventually(func(t *testcase.T) {
				assert.Equal(t, 1, phaser.Get(t).Len())
			})

			cancel()
			phaser.Get(t).Finish()

			assert.Within(t, time.Millisecond, func(ctx context.Context) {
				w.Wait()
			})

			assert.ErrorIs(t, gotErr, ctx.Err())
			assert.Equal(t, counter.Get(t)["foo"], 1)
			assert.Equal(t, counter.Get(t)["bar"], 1)
			assert.Equal(t, counter.Get(t)["baz"], 0)

			t.Log("and then re-execution should be possible, and continuing from where it was left")
			t.Log("when the same process entity is used")
			assert.NoError(t, rt.Get(t).Execute(t.Context(), &p))

			assert.Equal(t, counter.Get(t)["foo"], 1)
			assert.Equal(t, counter.Get(t)["bar"], 2, "expected that the failing bar is re-run")
			assert.Equal(t, counter.Get(t)["baz"], 1)
		})
	})
}
