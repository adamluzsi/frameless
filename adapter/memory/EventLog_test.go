package memory_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/meta"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/testcase"
)

var (
	_ memory.EventManager             = &memory.EventLog{}
	_ memory.EventManager             = &memory.EventLogTx{}
	_ comproto.OnePhaseCommitProtocol = &memory.EventLog{}
	_ meta.MetaAccessor               = &memory.EventLog{}
)

func TestMemory(t *testing.T) {
	SpecMemory{}.Spec(testcase.NewSpec(t))
}

type SpecMemory struct{}

func (spec SpecMemory) Spec(s *testcase.Spec) {
	spec.ctx().Bind(s)
	spec.memory().Bind(s)
	s.Describe(`.Add`, spec.SpecAdd)
}

func (spec SpecMemory) memory() testcase.Var[*memory.EventLog] {
	return testcase.Var[*memory.EventLog]{
		ID: `*memory.EventLog`,
		Init: func(t *testcase.T) *memory.EventLog {
			return memory.NewEventLog()
		},
	}
}

func (spec SpecMemory) memoryGet(t *testcase.T) *memory.EventLog {
	return spec.memory().Get(t)
}

func (spec SpecMemory) ctx() testcase.Var[context.Context] {
	return testcase.Var[context.Context]{
		ID: `context.Context`,
		Init: func(t *testcase.T) context.Context {
			return context.Background()
		},
	}
}

func (spec SpecMemory) ctxGet(t *testcase.T) context.Context {
	return spec.ctx().Get(t).(context.Context)
}

func (spec SpecMemory) SpecAdd(s *testcase.Spec) {
	type AddTestEvent struct{ V string }
	var (
		event = testcase.Let(s, func(t *testcase.T) interface{} {
			return AddTestEvent{V: `hello world`}
		})
		eventGet = func(t *testcase.T) memory.Event {
			return event.Get(t).(memory.Event)
		}
		subject = func(t *testcase.T) error {
			return spec.memoryGet(t).Append(spec.ctxGet(t), eventGet(t))
		}
	)

	s.When(`context is canceled`, func(s *testcase.Spec) {
		spec.ctx().Let(s, func(t *testcase.T) context.Context {
			c, cancel := context.WithCancel(context.Background())
			cancel()
			return c
		})

		s.Then(`atomic returns with context canceled error`, func(t *testcase.T) {
			assert.Must(t).ErrorIs(context.Canceled, subject(t))
		})
	})

	s.When(`during transaction`, func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			tx, err := spec.memoryGet(t).BeginTx(spec.ctxGet(t))
			assert.Must(t).NoError(err)
			spec.ctx().Set(t, tx)
		})

		s.Then(`Add will execute in the scope of transaction`, func(t *testcase.T) {
			assert.NoError(t, subject(t))
			assert.NotContains(t, spec.memoryGet(t).Events(), eventGet(t))
			assert.NoError(t, spec.memoryGet(t).CommitTx(spec.ctxGet(t)))
			assert.Contains(t, spec.memoryGet(t).Events(), eventGet(t))
		})
	})
}
