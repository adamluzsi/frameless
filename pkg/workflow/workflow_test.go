package workflow_test

import (
	"context"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
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
		assert.Equal[any](t, p.Variables.Get("foo-val"), fooOut)
		assert.Equal[any](t, p.Variables.Get("bar-val"), barOut)

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
			&workflow.ExecuteParticipant{
				ID: "flaky",
				//TODO: retry integration maybe?
			},
		}

		r := workflow.Runtime{
			Participants: participants,
		}

		var p workflow.Process

		assert.ErrorIs(t, expectedFlakyErr, pdef.Execute(r.Context(t.Context()), &p))
		assert.NotEmpty(t, p.Events)

		assert.NoError(t, pdef.Execute(r.Context(t.Context()), &p))
		assert.Equal[any](t, p.Variables.Get("foo-val"), fooOut)
		assert.Equal[any](t, p.Variables.Get("bar-val"), barOut)
		assert.Equal(t, ranCount["foo"], 1)
		assert.Equal(t, ranCount["bar"], 1)
		assert.Equal(t, ranCount["baz"], 1)
		assert.Equal(t, ranCount["flaky"], 2)
	})
}

/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// func Example() {

// 	rt := workflow.Runtime{
// 		Participants: workflow.ParticipantMapping{
// 			"foo": func(ctx context.Context, s *workflow.State) error {
// 				s.Variables.Set("foo", 42)
// 				return nil
// 			},
// 			"bar": func(ctx context.Context, s *workflow.State) error {
// 				s.Variables.Set("bar", 24)
// 				return nil
// 			},

// 			"then": func(ctx context.Context, s *workflow.State) error {
// 				return nil
// 			},
// 			"else": func(ctx context.Context, s *workflow.State) error {
// 				return nil
// 			},
// 		},
// 	}

// 	userDefinedWorkflowDefinition := &workflow.Sequence{
// 		workflow.PID("foo"),
// 		workflow.PID("bar"),
// 		&workflow.If{
// 			Cond: workflow.NewConditionTemplate(".foo <= .bar"),
// 			Then: workflow.PID("then"),
// 			Else: workflow.PID("else"),
// 		},
// 		&workflow.Concurrence{},
// 	}

// 	_ = userDefinedWorkflowDefinition.Execute(context.Background(), rt, &workflow.State{})
// }

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
	assert.Error(t, execBar.Execute(ctx0, &workflow.Process{}))

	ctx2 := workflow.ContextWithParticipants(ctx1, workflow.Participants{"bar": func(ctx context.Context) error { return nil }})
	assert.NoError(t, execFoo.Execute(ctx1, &workflow.Process{}))
	assert.NoError(t, execFoo.Execute(ctx2, &workflow.Process{}))
	assert.Error(t, execBar.Execute(ctx1, &workflow.Process{}))
	assert.NoError(t, execBar.Execute(ctx2, &workflow.Process{}))
}
