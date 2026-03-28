package workflow_test

import (
	"context"

	"go.llib.dev/frameless/pkg/workflow"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/let"
)

type StubParticipant struct {
	CallCount int
	Stub      func(ctx context.Context, p *workflow.Process) error
	Cond      func(ctx context.Context, p *workflow.Process) (bool, error)
	Err       error

	last *struct {
		ctx   context.Context
		state *workflow.Process
	}
}

func (stub *StubParticipant) LastExecutedWith() (context.Context, *workflow.Process, bool) {
	if stub.last == nil {
		return nil, nil, false
	}
	return stub.last.ctx, stub.last.state, true
}

func (stub *StubParticipant) Execute(ctx context.Context, s *workflow.Process) error {
	stub.CallCount++
	if stub.Stub != nil {
		return stub.Stub(ctx, s)
	}
	stub.last = &struct {
		ctx   context.Context
		state *workflow.Process
	}{
		ctx:   ctx,
		state: s,
	}
	return stub.Err
}

func (stub *StubParticipant) Evaluate(ctx context.Context, s *workflow.Process) (_ bool, _ error) {
	stub.CallCount++
	if stub.Cond != nil {
		return stub.Cond(ctx, s)
	}
	stub.last = &struct {
		ctx   context.Context
		state *workflow.Process
	}{
		ctx:   ctx,
		state: s,
	}
	return stub.Err == nil, stub.Err
}

type C struct {
	Context testcase.Var[context.Context]
	Process testcase.Var[*workflow.Process]
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
			"/dev/null": func(ctx context.Context, s *workflow.Process) error {
				return nil
			},
		})
		return ctx
	})

	c.Process = let.Var(s, func(t *testcase.T) *workflow.Process {
		return NewRandomState(t)
	})

	return c
}

func NewRandomState(t *testcase.T) *workflow.Process {
	var s = workflow.Process{}
	t.Random.Repeat(1, 3, func() {
		s.Variables.Set(workflow.VariableKey(t.Random.String()), t.Random.Int())
	})
	return &s
}
