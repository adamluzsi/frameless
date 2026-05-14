package wftesting

import (
	"context"

	"errors"
	"sync"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfschedule"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

type Stub struct {
	StubExecute  func(ctx context.Context, p *workflow.Process) error
	StubEvaluate func(ctx context.Context, p *workflow.Process) (bool, error)
}

var _ workflow.Definition = (*Stub)(nil)

func (stub Stub) Execute(ctx context.Context, p *workflow.Process) error {
	if stub.StubExecute != nil {
		return stub.StubExecute(ctx, p)
	}
	return nil
}

var _ workflow.Condition = (*Stub)(nil)

func (stub Stub) Evaluate(ctx context.Context, p *workflow.Process) (bool, error) {
	if stub.StubEvaluate != nil {
		return stub.StubEvaluate(ctx, p)
	}
	return true, nil
}

type StubParticipant struct {
	Err error

	m          sync.RWMutex
	last       *StubParticipantFuncArg
	_CallCount int
}

type StubParticipantFuncArg struct {
	Context context.Context
}

func (stub *StubParticipant) Last() (context.Context, bool) {
	stub.m.RLock()
	defer stub.m.RUnlock()
	if stub.last == nil {
		return nil, false
	}
	return stub.last.Context, true
}

func (stub *StubParticipant) CallCount() int {
	stub.m.RLock()
	defer stub.m.RUnlock()
	return stub._CallCount
}

func (stub *StubParticipant) Func(ctx context.Context) error {
	stub.m.Lock()
	defer stub.m.Unlock()
	stub._CallCount++
	stub.last = &StubParticipantFuncArg{Context: ctx}
	return stub.Err
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
			return workflow.ParticipantID(t.Random.Domain())
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

func (c *C) LetContext(s *testcase.Spec) testcase.Var[context.Context] {
	return let.Var(s, func(t *testcase.T) context.Context {
		return c.Runtime.Get(t).Context(t.Context())
	})
}

func (c *C) LetStub(s *testcase.Spec, pid testcase.Var[workflow.ParticipantID]) testcase.Var[*StubParticipant] {
	s.H().Helper()

	stub := let.Var(s, func(t *testcase.T) *StubParticipant {
		return &StubParticipant{}
	})

	c.Participants.Let(s, func(t *testcase.T) workflow.Participants {
		ps := c.Participants.Super(t)
		if ps == nil {
			ps = make(workflow.Participants)
		}
		ps[pid.Get(t)] = stub.Get(t).Func
		return ps
	})

	return stub
}

func LetProcessWithDefinition[Definition workflow.Definition](s *testcase.Spec, def testcase.Var[Definition]) testcase.Var[*workflow.Process] {
	return let.Var(s, func(t *testcase.T) *workflow.Process {
		var p workflow.Process
		p.Definition = def.Get(t)
		return &p
	})
}

func LetProcess(s *testcase.Spec) testcase.Var[*workflow.Process] {
	return let.Var(s, func(t *testcase.T) *workflow.Process {
		return &workflow.Process{}
	})
}

// C is a common dependencies often needed for workflow related tests
type C struct {
	Process testcase.Var[*workflow.Process]

	Runtime      testcase.Var[workflow.Runtime]
	Participants testcase.Var[workflow.Participants]
	Conditions   testcase.Var[workflow.Conditions]
	ContextSetup testcase.Var[[]func(context.Context) context.Context]

	Scheduler          testcase.Var[*wfschedule.Scheduler]
	SchedulerRunErr    testcase.Var[error]
	ProcessSignalQueue testcase.Var[*memory.Queue[wfschedule.ProcessSignal]]
	ProcessRepository  testcase.Var[*memory.Repository[workflow.Process, workflow.ProcessID]]
}

func LetC(s *testcase.Spec) C {
	s.H().Helper()

	var c C

	c.Participants = let.Var(s, func(t *testcase.T) workflow.Participants {
		return workflow.Participants{
			"/dev/null": func(ctx context.Context, s *workflow.Process) error {
				return nil
			},
		}
	})

	c.Conditions = let.Var(s, func(t *testcase.T) workflow.Conditions {
		return workflow.Conditions{
			"/dev/null": func(ctx context.Context, p *workflow.Process) (bool, error) {
				return false, nil
			},
		}
	})

	c.ProcessSignalQueue = let.Var(s, func(t *testcase.T) *memory.Queue[wfschedule.ProcessSignal] {
		return &memory.Queue[wfschedule.ProcessSignal]{}
	})

	c.ProcessRepository = let.Var(s, func(t *testcase.T) *memory.Repository[workflow.Process, workflow.ProcessID] {
		return &memory.Repository[workflow.Process, workflow.ProcessID]{}
	})

	c.SchedulerRunErr = let.VarOf[error](s, nil)

	c.Scheduler = let.Var(s, func(t *testcase.T) *wfschedule.Scheduler {
		var sch = &wfschedule.Scheduler{
			Runtime:            pointer.Of(c.Runtime.Get(t)),
			ProcessSignalQueue: c.ProcessSignalQueue.Get(t),
			ProcessRepository:  c.ProcessRepository.Get(t),
		}
		go func() {
			var err = sch.Run(t.Context())
			if ctxErr := t.Context().Err(); ctxErr != nil && errors.Is(err, ctxErr) {
				return
			}
			c.SchedulerRunErr.Set(t, err)
		}()
		return sch
	})

	c.ContextSetup = let.Var(s, func(t *testcase.T) []func(context.Context) context.Context {
		return workflow.ContextSetup{}
	})

	c.Runtime = let.Var(s, func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Participants: c.Participants.Get(t),
			Conditions:   c.Conditions.Get(t),
			ContextSetup: c.ContextSetup.Get(t),
		}
	})

	c.Process = LetProcess(s)

	return c
}

func ThenProcessIsCompleted(s *testcase.Spec, process testcase.Var[*workflow.Process], act func(t *testcase.T)) {
	s.Then("the workflow process is completed", func(t *testcase.T) {
		act(t)

		assert.True(t, workflow.IsCompleted(process.Get(t)))
	})
}

func ThenProcessIsNotCompleted(s *testcase.Spec, process testcase.Var[*workflow.Process], act func(t *testcase.T)) {
	s.Then("the workflow process is not completed", func(t *testcase.T) {
		act(t)

		assert.False(t, workflow.IsCompleted(process.Get(t)))
	})
}
