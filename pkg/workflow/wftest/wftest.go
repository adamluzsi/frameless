package wftest

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"

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
		var makeProcessID = func() workflow.ProcessID {
			return MakeProcessID(t)
		}
		pid := random.Unique(makeProcessID, processIDs.Get(t)...)
		testcase.Append(t, processIDs, pid)
		return pid
	})
}

func LetValue(s *testcase.Spec) testcase.Var[any] {
	return let.Var(s, func(t *testcase.T) any {
		return random.Pick[func() any](t.Random,
			func() any { return t.Random.String() },
			func() any { return t.Random.Int() },
			func() any { return t.Random.Float32() },
			func() any { return t.Random.Float64() },
			func() any { return t.Random.Time() },
			func() any { return t.Random.UUID() },
		)()
	})
}

func LetParticipant[Func any](s *testcase.Spec, mk func(t *testcase.T) Func) (testcase.Var[Func], testcase.Var[workflow.ParticipantID]) {
	var participantID = LetParticipantID(s)
	return LetParticipantWithID(s, participantID, mk), participantID
}

func LetParticipantWithID[Func any](s *testcase.Spec, pid testcase.Var[workflow.ParticipantID], mk func(t *testcase.T) Func) testcase.Var[Func] {
	typ := reflect.TypeFor[Func]()
	if typ.Kind() != reflect.Func {
		panic(fmt.Sprintf("LetParticipant expected Func but got %s", typ.String()))
	}
	p := let.Var(s, func(t *testcase.T) Func {
		return mk(t)
	})
	Participants.Let(s, func(t *testcase.T) workflow.Participants {
		ps := Participants.Super(t)
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

var Participants = testcase.Var[workflow.Participants]{
	ID: "workflow Participants",
	Init: func(t *testcase.T) workflow.Participants {
		return workflow.Participants{
			"/dev/null": func(ctx context.Context) error {
				return nil
			},
		}
	},
}

var Conditions = testcase.Var[workflow.Conditions]{
	ID: "workflow Conditions",
	Init: func(t *testcase.T) workflow.Conditions {
		return workflow.Conditions{
			"/dev/null": func(ctx context.Context) (bool, error) {
				return false, nil
			},
		}
	},
}

var EventRepository = testcase.Var[*memory.WorkflowEventRepository]{
	ID: "workflow EventRepository",
	Init: func(t *testcase.T) *memory.WorkflowEventRepository {
		return &memory.WorkflowEventRepository{}
	},
}

var ProcessExecutionQueue = testcase.Var[*memory.WorkflowProcessExecutionQueue]{
	ID: "workflow ProcessExecutionQueue",
	Init: func(t *testcase.T) *memory.WorkflowProcessExecutionQueue {
		return &memory.WorkflowProcessExecutionQueue{}
	},
}

var ProcessChangeBroadcast = testcase.Var[*memory.WorkflowProcessChangeBroadcast]{
	ID: "workflow ProcessChangeBroadcast",
	Init: func(t *testcase.T) *memory.WorkflowProcessChangeBroadcast {
		return &memory.WorkflowProcessChangeBroadcast{}
	},
}

var ProcessLocks = testcase.Var[*memory.WorkflowProcessLocks]{
	ID: "workflow ProcessLocks",
	Init: func(t *testcase.T) *memory.WorkflowProcessLocks {
		return &memory.WorkflowProcessLocks{}
	},
}

var ErrRuntimeRun = testcase.Var[error]{
	ID: "workflow ErrRuntimeRun",
}

var Runtime = testcase.Var[workflow.Runtime]{
	ID: "workflow Runtime",
	Init: func(t *testcase.T) workflow.Runtime {
		return workflow.Runtime{
			Participants:       Participants.Get(t),
			Conditions:         Conditions.Get(t),
			Events:             EventRepository.Get(t),
			Queue:              ProcessExecutionQueue.Get(t),
			Changes:            ProcessChangeBroadcast.Get(t),
			Locks:              ProcessLocks.Get(t),
			RetryStrategy:      noFaultTolerance{},
			WaitTime:           time.Nanosecond,
			NumQueueSubscriber: 2,
			Codec:              wfjson.NewCodec(),
		}
	},
	Before: func(t *testcase.T, v testcase.Var[workflow.Runtime]) {
		var rt = v.Get(t)
		t.Go(func(ctx context.Context) error {
			var err = rt.Run(ctx)
			if ctx.Err() != nil {
				return nil
			}
			ErrRuntimeRun.Set(t, err)
			return nil
		})
	},
	Deps: testcase.Vars{
		Participants,
		Conditions,
		EventRepository,
		ProcessExecutionQueue,
		ProcessChangeBroadcast,
		ProcessLocks,
	},
}

// C is a common dependencies often needed for workflow related tests
type C struct {
	ProcessID  testcase.Var[workflow.ProcessID]
	Definition testcase.Var[workflow.Definition]

	Runtime      testcase.Var[workflow.Runtime]
	Participants testcase.Var[workflow.Participants]
	Conditions   testcase.Var[workflow.Conditions]

	ErrRuntimeRun testcase.Var[error]

	EventRepository        testcase.Var[*memory.WorkflowEventRepository]
	ProcessExecutionQueue  testcase.Var[*memory.WorkflowProcessExecutionQueue]
	ProcessChangeBroadcast testcase.Var[*memory.WorkflowProcessChangeBroadcast]

	ProcessLocks testcase.Var[*memory.WorkflowProcessLocks]
}

func LetC(s *testcase.Spec) C {
	s.H().Helper()

	var c C

	c.Participants = Participants.Bind(s)
	c.Conditions = Conditions.Bind(s)
	c.EventRepository = EventRepository.Bind(s)
	c.ProcessExecutionQueue = ProcessExecutionQueue.Bind(s)
	c.ProcessChangeBroadcast = ProcessChangeBroadcast.Bind(s)
	c.ProcessLocks = ProcessLocks.Bind(s)
	c.ErrRuntimeRun = ErrRuntimeRun
	c.Runtime = Runtime.Bind(s)

	c.ProcessID = LetProcessID(s)

	c.Definition = let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.SetVar{Name: "answer", Value: 42}
	})

	s.Before(func(t *testcase.T) {
		assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), c.ProcessID.Get(t), c.Definition.Get(t)))
	})

	return c
}

func (c *C) ActExecute(t *testcase.T) error {
	return c.ActExecuteDefinition(t, t.Context(), c.ProcessID.Get(t), c.Definition.Get(t))
}

func (c *C) ActExecuteDefinition(t *testcase.T, ctx context.Context, processID workflow.ProcessID, definition workflow.Definition) error {
	assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), processID, definition))
	testcase.OnFail(t, func() {
		t.Log("definition:")
		t.LogPretty(definition)
	})
	return c.Runtime.Get(t).Execute(ctx, processID)
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
	var isCompleted, err = workflow.IsCompleted(t.Context(), c.EventRepository.Get(t), pid)
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
		isCompleted, err := workflow.IsCompleted(t.Context(), EventsRepository, processID)
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

func MakeEventID(tb testing.TB) workflow.EventID {
	id, err := workflow.MakeEventID()
	assert.NoError(tb, err)
	return id
}

func MakeProcessID(tb testing.TB) workflow.ProcessID {
	id, err := workflow.MakeProcessID()
	assert.NoError(tb, err)
	return id
}
