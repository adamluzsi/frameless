package workflow_test

import (
	"context"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

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

func Test_smoke(tt *testing.T) {
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
		assert.NoError(t, pdef.Execute(r.Context(t.Context()), r, &p))
		assert.Equal[any](t, p.Variables.Get("foo-val"), fooOut)
		assert.Equal[any](t, p.Variables.Get("bar-val"), barOut)

	})

	s.Test("definition idempotency", func(t *testcase.T) {
		var (
			fooOut = t.Random.String()
			barOut = t.Random.Int()

			failOnce sync.Once
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

				var err error
				failOnce.Do(func() { err = t.Random.Error() })
				return barOut, err
			},
			"baz": func(ctx context.Context, s string, n int) error {
				inc("baz")
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
			&workflow.ExecuteParticipant{
				ID: "flaky",
			},
		}

		r := workflow.Runtime{
			Participants: participants,
		}

		_ = r
	})

	t := testcase.NewT(tt)

	assert.NoError(t, pdef.Validate(r.Context(t.Context())), "expected that the process definition is valid")

	var state workflow.State

	assert.NoError(t, r.Execute(t.Context(), pdef, &state))
}

type StubParticipant struct {
	CallCount int
	Stub      func(ctx context.Context, s *workflow.State) error
	Cond      func(ctx context.Context, s *workflow.State) (bool, error)
	Err       error

	last *struct {
		ctx   context.Context
		state *workflow.State
	}
}

func (stub *StubParticipant) LastExecutedWith() (context.Context, *workflow.State, bool) {
	if stub.last == nil {
		return nil, nil, false
	}
	return stub.last.ctx, stub.last.state, true
}

func (stub *StubParticipant) Execute(ctx context.Context, s *workflow.State) error {
	stub.CallCount++
	if stub.Stub != nil {
		return stub.Stub(ctx, s)
	}
	stub.last = &struct {
		ctx   context.Context
		state *workflow.State
	}{
		ctx:   ctx,
		state: s,
	}
	return stub.Err
}

func (stub *StubParticipant) Evaluate(ctx context.Context, s *workflow.State) (_ bool, _ error) {
	stub.CallCount++
	if stub.Cond != nil {
		return stub.Cond(ctx, s)
	}
	stub.last = &struct {
		ctx   context.Context
		state *workflow.State
	}{
		ctx:   ctx,
		state: s,
	}
	return stub.Err == nil, stub.Err
}

type C struct {
	Context testcase.Var[context.Context]
	State   testcase.Var[*workflow.State]
}

func (c *C) LetStub(s *testcase.Spec, pid workflow.ParticipantID) testcase.Var[*StubParticipant] {
	s.H().Helper()

	stub := let.Var(s, func(t *testcase.T) *StubParticipant {
		return &StubParticipant{}
	})

	c.Context.Let(s, func(t *testcase.T) context.Context {
		og := c.Context.Super(t)
		return workflow.ContextWithParticipants(og,
			workflow.Participants{pid: stub.Get(t)})
	})

	return stub
}

func letC(s *testcase.Spec) C {
	var c C

	c.Context = let.Var(s, func(t *testcase.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		t.Defer(cancel)

		ctx = workflow.ContextWithParticipants(ctx, workflow.Participants{
			"/dev/null": func(ctx context.Context, s *workflow.State) error {
				return nil
			},
		})
		return ctx
	})

	c.State = let.Var(s, func(t *testcase.T) *workflow.State {
		return NewRandomState(t)
	})

	return c
}

func NewRandomState(t *testcase.T) *workflow.State {
	var s = workflow.State{}
	t.Random.Repeat(1, 3, func() {
		s.Variables.Set(t.Random.String(), t.Random.Int())
	})
	return &s
}
