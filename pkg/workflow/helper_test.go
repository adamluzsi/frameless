package workflow_test

import (
	"context"

	"go.llib.dev/frameless/pkg/workflow"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

type StubParticipant struct {
	CallCount int
	Err       error
	last      *StubParticipantFuncArg
}

type StubParticipantFuncArg struct {
	Context context.Context
}

func (stub *StubParticipant) LastExecutedWith() (context.Context, bool) {
	if stub.last == nil {
		return nil, false
	}
	return stub.last.Context, true
}

func (stub *StubParticipant) Func(ctx context.Context) error {
	stub.CallCount++
	stub.last = &StubParticipantFuncArg{Context: ctx}
	return stub.Err
}

type C struct {
	Context testcase.Var[context.Context]
	Process testcase.Var[*workflow.Process]

	Runtime      testcase.Var[workflow.Runtime]
	Participants testcase.Var[workflow.Participants]
}

var pids = testcase.Var[[]workflow.ParticipantID]{
	ID: "workflow participant IDs generated with LetParticipantID",
	Init: func(t *testcase.T) []workflow.ParticipantID {
		return make([]workflow.ParticipantID, 0)
	},
}

func LetParticipantID(s *testcase.Spec) testcase.Var[workflow.ParticipantID] {
	return let.Var(s, func(t *testcase.T) workflow.ParticipantID {
		pid := random.Unique(func() workflow.ParticipantID {
			return workflow.ParticipantID(t.Random.String())
		}, pids.Get(t)...)
		testcase.Append(t, pids, pid)
		return pid
	})
}

func LetParticipant[Func any](s *testcase.Spec, c C, pid testcase.Var[workflow.ParticipantID], mk func(t *testcase.T) Func) testcase.Var[Func] {
	p := let.Var(s, func(t *testcase.T) Func {
		return mk(t)
	})
	c.Participants.Let(s, func(t *testcase.T) workflow.Participants {
		ps := c.Participants.Super(t)
		if ps == nil {
			ps = make(workflow.Participants)
		}
		ps[pid.Get(t)] = p.Get(t)
		return ps
	})
	return p
}

func (c *C) LetStub(s *testcase.Spec, pid workflow.ParticipantID) testcase.Var[*StubParticipant] {
	s.H().Helper()

	stub := let.Var(s, func(t *testcase.T) *StubParticipant {
		return &StubParticipant{}
	})

	c.Participants.Let(s, func(t *testcase.T) workflow.Participants {
		ps := c.Participants.Super(t)
		if ps == nil {
			ps = make(workflow.Participants)
		}
		ps[pid] = stub.Get(t).Func
		return ps
	})

	return stub
}

func letC(s *testcase.Spec) C {
	var c C

	c.Participants = let.Var(s, func(t *testcase.T) workflow.Participants {
		return workflow.Participants{
			"/dev/null": func(ctx context.Context, s *workflow.Process) error {
				return nil
			},
		}
	})

	c.Runtime = let.Var(s, func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Participants: c.Participants.Get(t),
		}
	})

	c.Context = let.Var(s, func(t *testcase.T) context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		t.Defer(cancel)
		return c.Runtime.Get(t).Context(ctx)
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
