package wftest

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"errors"
	"sync"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/port/guard"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

type Stub struct {
	StubExecute  func(ctx context.Context, pid workflow.ProcessID) error
	StubEvaluate func(ctx context.Context, pid workflow.ProcessID) (bool, error)
}

var _ workflow.Definition = (*Stub)(nil)

func (stub Stub) Error() string { return "wftest.Stub" }

func (stub Stub) Execute(ctx context.Context, pid workflow.ProcessID) error {
	if stub.StubExecute != nil {
		return stub.StubExecute(ctx, pid)
	}
	return nil
}

var _ workflow.Condition = (*Stub)(nil)

func (stub Stub) Evaluate(ctx context.Context, pid workflow.ProcessID) (bool, error) {
	if stub.StubEvaluate != nil {
		return stub.StubEvaluate(ctx, pid)
	}
	return true, nil
}

// StubParticipant
//
// Deprecated: use Stub instead
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

var participantIDs = testcase.Var[[]workflow.ParticipantID]{
	ID: "workflow participant IDs generated with LetParticipantID",
	Init: func(t *testcase.T) []workflow.ParticipantID {
		return make([]workflow.ParticipantID, 0)
	},
}

func LetParticipantID(s *testcase.Spec) testcase.Var[workflow.ParticipantID] {
	return let.Var(s, func(t *testcase.T) workflow.ParticipantID {
		id := random.Unique(func() workflow.ParticipantID {
			return workflow.ParticipantID(t.Random.Domain())
		}, participantIDs.Get(t)...)
		testcase.Append(t, participantIDs, id)
		return id
	})
}

var processIDs = testcase.Var[[]workflow.ProcessID]{
	ID: "workflow participant IDs generated with LetProcessID",
	Init: func(t *testcase.T) []workflow.ProcessID {
		return make([]workflow.ProcessID, 0)
	},
}

func LetProcessID(s *testcase.Spec) testcase.Var[workflow.ProcessID] {
	return let.Var(s, func(t *testcase.T) workflow.ProcessID {
		pid := random.Unique(func() workflow.ProcessID {
			id, err := workflow.MakeProcessID()
			assert.NoError(t, err)
			return id
		}, processIDs.Get(t)...)
		testcase.Append(t, processIDs, pid)
		return pid
	})
}

func LetParticipant[Func any](s *testcase.Spec, c C, mk func(t *testcase.T) Func) (testcase.Var[Func], testcase.Var[workflow.ParticipantID]) {
	var participantID = LetParticipantID(s)
	return LetParticipantWithID(s, c, participantID, mk), participantID
}

func LetParticipantWithID[Func any](s *testcase.Spec, c C, pid testcase.Var[workflow.ParticipantID], mk func(t *testcase.T) Func) testcase.Var[Func] {
	typ := reflect.TypeFor[Func]()
	if typ.Kind() != reflect.Func {
		panic(fmt.Sprintf("LetParticipant expected Func but got %s", typ.String()))
	}
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

// LetStubParticipant
//
// Deprecated: Use LetParticipant instead
func (c *C) LetStubParticipant(s *testcase.Spec, pid testcase.Var[workflow.ParticipantID]) testcase.Var[*StubParticipant] {
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

// LetProcessWithDefinition returns a testcase.Var[workflow.ProcessID] that is
// initialised once per test case by delegating to Runtime.Bind, which records
// a UseDefinitionEvent in the runtime's EventsRepository. The returned ID is
// ready for Execute to pick up.
//
// Using Runtime.Bind instead of writing directly to the events repo keeps the
// test helper thin and lets the production code own the "associate this
// Definition with this ProcessID" operation. Process is intentionally
// stateless — the current definition is always read from the event history.
func LetProcessWithDefinition[Definition workflow.Definition](s *testcase.Spec, c C, def testcase.Var[Definition]) testcase.Var[workflow.ProcessID] {
	return let.Var(s, func(t *testcase.T) workflow.ProcessID {
		id, err := workflow.MakeProcessID()
		assert.NoError(t, err)
		assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), id, def.Get(t)))
		return id
	})
}

// LetProcess returns a testcase.Var[workflow.ProcessID] generating a fresh
// ProcessID per test case. Use it together with Runtime.Bind (or manually
// seed the event history) when a Process needs to be associated with a
// definition.
func LetProcess(s *testcase.Spec) testcase.Var[workflow.ProcessID] {
	return let.Var(s, func(t *testcase.T) workflow.ProcessID {
		id, err := workflow.MakeProcessID()
		assert.NoError(t, err)
		return id
	})
}

// C is a common dependencies often needed for workflow related tests
type C struct {
	ProcessID  testcase.Var[workflow.ProcessID]
	Definition testcase.Var[workflow.Definition]

	Runtime      testcase.Var[workflow.Runtime]
	Participants testcase.Var[workflow.Participants]
	Conditions   testcase.Var[workflow.Conditions]
	ContextSetup testcase.Var[[]func(context.Context) context.Context]

	ErrRuntimeRun testcase.Var[error]

	EventRepository        testcase.Var[*memory.WorkflowEventRepository]
	ProcessExecutionQueue  testcase.Var[*memory.WorkflowProcessExecutionQueue]
	ProcessChangeBroadcast testcase.Var[*memory.WorkflowProcessChangeBroadcast]

	ProcessLockers testcase.Var[*memory.LockerFactory[workflow.ProcessID, guard.NonBlockingLocker]]
}

func LetC(s *testcase.Spec) C {
	s.H().Helper()

	var c C

	c.Participants = let.Var(s, func(t *testcase.T) workflow.Participants {
		return workflow.Participants{
			"/dev/null": func(ctx context.Context) error {
				return nil
			},
		}
	})

	c.Conditions = let.Var(s, func(t *testcase.T) workflow.Conditions {
		return workflow.Conditions{
			"/dev/null": func(ctx context.Context) (bool, error) {
				return false, nil
			},
		}
	})

	c.ProcessExecutionQueue = let.Var(s, func(t *testcase.T) *memory.WorkflowProcessExecutionQueue {
		return &memory.WorkflowProcessExecutionQueue{}
	})

	c.EventRepository = let.Var(s, func(t *testcase.T) *memory.WorkflowEventRepository {
		return &memory.WorkflowEventRepository{}
	})

	c.ProcessChangeBroadcast = let.Var(s, func(t *testcase.T) *memory.WorkflowProcessChangeBroadcast {
		return &memory.WorkflowProcessChangeBroadcast{}
	})

	c.ErrRuntimeRun = let.VarOf[error](s, nil)

	c.ContextSetup = let.Var(s, func(t *testcase.T) []func(context.Context) context.Context {
		return workflow.ContextSetup{}
	})

	c.ProcessLockers = let.Var(s, func(t *testcase.T) *memory.LockerFactory[workflow.ProcessID, guard.NonBlockingLocker] {
		return &memory.LockerFactory[workflow.ProcessID, guard.NonBlockingLocker]{}
	})

	c.Runtime = let.Var(s, func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Participants:           c.Participants.Get(t),
			Conditions:             c.Conditions.Get(t),
			ContextSetup:           c.ContextSetup.Get(t),
			Events:                 c.EventRepository.Get(t),
			ProcessExecutionQueue:  c.ProcessExecutionQueue.Get(t),
			ProcessChangeBroadcast: c.ProcessChangeBroadcast.Get(t),
			ProcessLockers:         c.ProcessLockers.Get(t),
			RetryStrategy:          noFaultTolerance{},
			WaitTime:               time.Nanosecond,
			Codec:                  wfjson.NewCodec(),
		}
	})
	c.Runtime.Before = func(t *testcase.T, v testcase.Var[workflow.Runtime]) {
		go func() {
			var ctx = t.Context()
			var err = v.Get(t).Run(ctx)
			if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(err, ctxErr) {
				return
			}
			c.ErrRuntimeRun.Set(t, err)
		}()
	}

	c.ProcessID = LetProcessID(s)

	c.Definition = let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.SetVar{Key: "answer", Value: 42}
	})

	s.Before(func(t *testcase.T) {
		assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), c.ProcessID.Get(t), c.Definition.Get(t)))
	})

	return c
}

func (c *C) ActExecute(t *testcase.T) error {
	return c.ActExecuteDefinition(t, c.ProcessID.Get(t), c.Definition.Get(t))
}

func (c *C) ActExecuteDefinition(t *testcase.T, processID workflow.ProcessID, definition workflow.Definition) error {
	assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, definition))
	testcase.OnFail(t, func() {
		t.Log("definition:")
		t.LogPretty(definition)
	})
	return c.Runtime.Get(t).Execute(t.Context(), processID)
}

// ProcessEvents returns the recorded event history of the given Process by reading it
// from the runtime's EventsRepository, which is the single source of truth.
func (c *C) ProcessEvents(t *testcase.T, pid workflow.ProcessID) []workflow.Event {
	t.Helper()
	got, err := iterkit.CollectE(c.EventRepository.Get(t).FindByProcessID(t.Context(), pid))
	assert.NoError(t, err)
	return got
}

// IsCompleted reports whether the given Process has a completion event recorded
// in the runtime's EventsRepository.
func (c *C) IsCompleted(t *testcase.T, pid workflow.ProcessID) bool {
	t.Helper()
	var complete = workflow.Complete{ProcessID: pid}
	var isCompleted, err = complete.IsCompleted(t.Context(), c.EventRepository.Get(t))
	assert.NoError(t, err)
	return isCompleted
}

func (c *C) ThenProcessIsCompleted(s *testcase.Spec, process testcase.Var[workflow.ProcessID], act func(t *testcase.T)) {
	s.H().Helper()
	s.Then("the workflow process is completed", func(t *testcase.T) {
		act(t)

		assert.True(t, c.IsCompleted(t, process.Get(t)))
	})
}

func (c *C) ThenProcessIsNotCompleted(s *testcase.Spec, process testcase.Var[workflow.ProcessID], act func(t *testcase.T)) {
	s.Then("the workflow process is not completed", func(t *testcase.T) {
		act(t)

		assert.False(t, c.IsCompleted(t, process.Get(t)))
	})
}

const deadline = time.Second

func (c *C) ProcessCompletionIs(t *testcase.T, processID workflow.ProcessID, done bool) {
	t.Helper()
	var EventsRepository = c.EventRepository.Get(t)

	assert.Eventually(t, deadline, func(t testing.TB) {
		isCompleted, err := workflow.Complete{ProcessID: processID}.IsCompleted(t.Context(), EventsRepository)
		assert.NoError(t, err)
		assert.Equal(t, isCompleted, done)
	})
}

func (c *C) WaitForSpawn(t *testcase.T, parentID workflow.ProcessID) {
	t.Helper()
	var EventsRepository = c.EventRepository.Get(t)
	assert.Eventually(t, deadline, func(t testing.TB) {
		events, err := iterkit.CollectE(EventsRepository.FindByProcessID(t.Context(), parentID))
		assert.NoError(t, err)
		assert.NotEmpty(t, events)
		assert.OneOf(t, events, func(t testing.TB, e workflow.Event) {
			spawn, ok := e.(workflow.EventSpawn)
			assert.True(t, ok)
			assert.NotEmpty(t, spawn.ChildID)
			assert.Equal(t, spawn.ProcessID, parentID)
			assert.NotEmpty(t, spawn.Timestamp)
		}, "expected that one of the workflow events signaling about a sub workflow spawn event")
	})
}

func (c *C) ChildrenCompletionAre(tc *testcase.T, parentID workflow.ProcessID, done bool) {
	tc.Helper()
	assert.Eventually(tc, deadline, func(t testing.TB) {
		events, err := iterkit.CollectE(c.EventRepository.Get(tc).FindByProcessID(t.Context(), parentID))
		assert.NoError(t, err)

		var found bool
		for _, event := range events {
			spawn, ok := event.(workflow.EventSpawn)
			if !ok {
				continue
			}
			found = true

			assert.NotEmpty(t, spawn.ChildID)
			assert.Equal(t, spawn.ProcessID, parentID)
			assert.NotEmpty(t, spawn.Timestamp)

			assert.Equal(t, c.IsCompleted(tc, spawn.ChildID), done,
				assert.MessageF("expected that completion state is %v", done))
		}
		assert.True(t, found, "expected to find at least one child workflow")
	})
}

type noFaultTolerance struct{}

var _ resilience.RetryStrategy = noFaultTolerance{}

func (noFaultTolerance) ShouldTry(ctx context.Context, attempt resilience.RetryAttempt) bool {
	if attempt.FailureCount == 0 {
		return true
	}
	return false
}
