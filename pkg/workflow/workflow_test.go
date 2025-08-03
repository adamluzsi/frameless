package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/pp"
)

func Test_smoke(t *testing.T) {
	templateFuncMap := workflow.TemplateFuncMap{
		"isOK": func(v any) bool {
			return true
		},
	}

	participants := workflow.Participants{
		"foo": workflow.ParticipantFunc(func(ctx context.Context, r *workflow.State) error {
			return nil
		}),
		"bar": workflow.ParticipantFunc(func(ctx context.Context, r *workflow.State) error {
			return nil
		}),
		"baz": workflow.ParticipantFunc(func(ctx context.Context, r *workflow.State) error {
			return nil
		}),
		"qux": workflow.ParticipantFunc(func(ctx context.Context, r *workflow.State) error {
			return nil
		}),
	}

	var pdef workflow.Definition = &workflow.If{
		Cond: workflow.NewConditionTemplate(`eq .X "foo"`),
		Then: &workflow.Sequence{
			workflow.PID("foo"),
			workflow.PID("bar"),
			&workflow.If{
				Cond: workflow.NewConditionTemplate(`isOK .X`),
				Then: workflow.PID("qux"),
			},
		},
		Else: workflow.PID("baz"),
	}

	r := workflow.Runtime{
		Participants:    participants,
		TemplateFuncMap: templateFuncMap,
	}

	assert.NoError(t, pdef.Validate(r.Context(t.Context())), "expected that the process definition is valid")

	state := workflow.NewState()

	assert.NoError(t, r.Execute(t.Context(), pdef, state))
}

type StubParticipant struct {
	CallCount int
	Stub      func(ctx context.Context, s *workflow.State) error
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

type C struct {
	Context testcase.Var[context.Context]
	State   testcase.Var[*workflow.State]
	Stub    testcase.Var[*StubParticipant]

	stubs testcase.Var[map[workflow.ParticipantID]*StubParticipant]
}

func (c *C) LetStub(s *testcase.Spec, pid workflow.ParticipantID) testcase.Var[*StubParticipant] {
	stub := let.Var(s, func(t *testcase.T) *StubParticipant {
		return &StubParticipant{}
	})
	ctx := c.Context
	c.Context.Let(s, func(t *testcase.T) context.Context {
		og := ctx.Super(t)
		pp.PP(pid)
		return workflow.ContextWithParticipants(og, workflow.Participants{
			pid: stub.Get(t),
		})
	})
	return stub
}

func letC(s *testcase.Spec) C {
	var c C

	c.Context = let.Var(s, func(t *testcase.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		t.Defer(cancel)
		ctx = workflow.ContextWithParticipants(ctx, workflow.Participants{
			"/dev/null": workflow.ParticipantFunc(func(ctx context.Context, s *workflow.State) error {
				return nil
			}),
		})
		return ctx
	})

	c.State = let.Var(s, func(t *testcase.T) *workflow.State {
		return NewRandomState(t)
	})

	c.Stub = c.LetStub(s, "stub")

	return c
}

func NewRandomState(t *testcase.T) *workflow.State {
	var s = workflow.NewState()
	t.Random.Repeat(1, 3, func() {
		s.Variables[t.Random.String()] = t.Random.Int()
	})
	return s
}
