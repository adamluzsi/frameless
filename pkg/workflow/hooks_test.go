package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestDefer(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		eventRepository = let.Var(s, func(t *testcase.T) workflow.EventRepository {
			return &memory.WorkflowEventRepository{}
		})
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})
		// workflowContext is the context a participant or a definition receives
		// during the execution of the process: it carries the runtime and the process id.
		workflowContext = func(t *testcase.T, ctx context.Context) context.Context {
			ctx = workflow.Runtime{Events: eventRepository.Get(t)}.Context(ctx)
			return wftest.ContextWithProcessID(t, ctx, processID.Get(t))
		}
		ctx = let.Var(s, func(t *testcase.T) context.Context {
			c := workflowContext(t, t.Context())
			c = workflow.WithName(c, t.Random.UUID())
			return workflow.WithVarScope(c, t.Random.UUID())
		})
		def = let.Var(s, func(t *testcase.T) workflow.Definition {
			return workflow.Execute{Participant: workflow.ParticipantID(t.Random.UUID())}
		})
	)
	act := func(t *testcase.T) error {
		return workflow.Defer(ctx.Get(t), def.Get(t))
	}

	hooksOf := func(t *testcase.T, pid workflow.ProcessID) []workflow.EventDefer {
		t.Helper()
		return deferHooksOf(t, eventRepository.Get(t), pid)
	}

	s.Then("the hook is registered in the process's event log", func(t *testcase.T) {
		assert.NoError(t, act(t))

		hooks := hooksOf(t, processID.Get(t))
		assert.Equal(t, len(hooks), 1)
		hook := hooks[0]
		assert.NotEmpty(t, hook.EventID)
		assert.Equal(t, hook.ProcessID, processID.Get(t))
		assert.NotEmpty(t, hook.Timestamp)
		assert.Equal(t, hook.Definition, def.Get(t))
	})

	s.Then("the hook records where it was registered", func(t *testcase.T) {
		assert.NoError(t, act(t))

		hooks := hooksOf(t, processID.Get(t))
		assert.Equal(t, len(hooks), 1)
		assert.Equal(t, hooks[0].Path, workflow.CurrentPath(ctx.Get(t)))
		assert.Equal(t, hooks[0].VarScope, workflow.CurrentVarScope(ctx.Get(t)))
	})

	s.Then("the hook is not registered to other processes", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Empty(t, hooksOf(t, mustProcessID(t)))
	})

	s.Then("repeating the registration at the same step is idempotent, so a replay doesn't register it twice", func(t *testcase.T) {
		assert.NoError(t, act(t))
		assert.NoError(t, act(t))

		assert.Equal(t, len(hooksOf(t, processID.Get(t))), 1)
	})

	s.When("a hook is already registered at another step", func(s *testcase.Spec) {
		othDef := let.Var(s, func(t *testcase.T) workflow.Definition {
			return workflow.Execute{Participant: workflow.ParticipantID(t.Random.UUID())}
		})
		s.Before(func(t *testcase.T) {
			othCtx := workflow.WithName(workflowContext(t, t.Context()), t.Random.UUID())
			assert.Must(t).NoError(workflow.Defer(othCtx, othDef.Get(t)))
		})

		s.Then("both hooks are kept, in the order of their registration", func(t *testcase.T) {
			assert.NoError(t, act(t))

			hooks := hooksOf(t, processID.Get(t))
			assert.Equal(t, len(hooks), 2)
			assert.Equal(t, hooks[0].Definition, othDef.Get(t))
			assert.Equal(t, hooks[1].Definition, def.Get(t))
		})
	})

	s.When("the same step is re-entered under a different variable scope, like a loop iteration", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			sibling := workflow.WithVarScope(ctx.Get(t), t.Random.UUID())
			assert.Must(t).NoError(workflow.Defer(sibling, def.Get(t)))
		})

		s.Then("each scope registers its own hook", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, len(hooksOf(t, processID.Get(t))), 2)
		})
	})

	s.When("the definition is nil", func(s *testcase.Spec) {
		def.LetValue(s, nil)

		s.Then("it fails with a fatal error, as there is nothing to defer", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.ErrFatal)

			assert.Empty(t, hooksOf(t, processID.Get(t)))
		})
	})

	s.When("the context is canceled", func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			c, cancel := context.WithCancel(ctx.Super(t))
			cancel()
			return c
		})

		s.Then("it returns the context error and registers nothing", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), context.Canceled)

			assert.Empty(t, hooksOf(t, processID.Get(t)))
		})
	})

	s.When("the context is not a workflow execution context", func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			return workflow.WithName(t.Context(), t.Random.UUID())
		})

		s.Then("it fails, as there is no process to register the hook to", func(t *testcase.T) {
			assert.Error(t, act(t))

			assert.Empty(t, hooksOf(t, processID.Get(t)))
		})
	})

	s.When("the context has no process id", func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			return workflow.Runtime{Events: eventRepository.Get(t)}.Context(t.Context())
		})

		s.Then("it fails with a fatal error", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.ErrFatal)

			assert.Empty(t, hooksOf(t, processID.Get(t)))
		})
	})

	s.When("the context has no runtime", func(s *testcase.Spec) {
		ctx.Let(s, func(t *testcase.T) context.Context {
			return wftest.ContextWithProcessID(t, t.Context(), processID.Get(t))
		})

		s.Then("it fails with the missing runtime error", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.ErrNoContextRuntime)

			assert.Empty(t, hooksOf(t, processID.Get(t)))
		})
	})
}

func TestEventDeferHook(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		eventRepository = let.Var(s, func(t *testcase.T) workflow.EventRepository {
			return &memory.WorkflowEventRepository{}
		})
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})
		declaredPath = let.Var(s, func(t *testcase.T) workflow.Path {
			return workflow.Path{t.Random.UUID(), t.Random.UUID(), t.Random.UUID()}
		})
		declaredScope = let.Var(s, func(t *testcase.T) workflow.VarScope {
			return workflow.VarScope{t.Random.UUID(), t.Random.UUID()}
		})
		subject = let.Var(s, func(t *testcase.T) workflow.EventDefer {
			id, err := workflow.MakeEventID()
			assert.Must(t).NoError(err)
			return workflow.EventDefer{
				EventID:   id,
				ProcessID: processID.Get(t),
				Path:      declaredPath.Get(t),
				VarScope:  declaredScope.Get(t),
			}
		})
	)

	s.Describe("#Context", func(s *testcase.Spec) {
		ctx := let.Var(s, func(t *testcase.T) context.Context {
			return t.Context()
		})
		act := func(t *testcase.T) context.Context {
			return subject.Get(t).Context(ctx.Get(t))
		}

		s.Then("the hooked definition runs under the declaration's path, in a segment named after the hook's event id", func(t *testcase.T) {
			exp := append(workflow.Path{}, declaredPath.Get(t)...)
			exp = append(exp, subject.Get(t).EventID.String())
			assert.Equal(t, workflow.CurrentPath(act(t)), exp)
		})

		s.Then("the hooked definition runs under the declaration's variable scope, in a scope named after the hook's event id", func(t *testcase.T) {
			exp := append(workflow.VarScope{}, declaredScope.Get(t)...)
			exp = append(exp, subject.Get(t).EventID.String())
			assert.Equal(t, workflow.CurrentVarScope(act(t)), exp)
		})

		s.Then("the hook's own path and variable scope are left untouched", func(t *testcase.T) {
			_ = act(t)
			assert.Equal(t, subject.Get(t).Path, declaredPath.Get(t))
			assert.Equal(t, subject.Get(t).VarScope, declaredScope.Get(t))
		})

		s.When("multiple hooks are declared at the same location", func(s *testcase.Spec) {
			othHook := let.Var(s, func(t *testcase.T) workflow.EventDefer {
				id, err := workflow.MakeEventID()
				assert.Must(t).NoError(err)
				h := subject.Get(t)
				h.EventID = id
				return h
			})

			s.Then("each hook runs on a distinct path, so their idempotent steps don't collide", func(t *testcase.T) {
				assert.NotEqual(t, workflow.CurrentPath(act(t)), workflow.CurrentPath(othHook.Get(t).Context(ctx.Get(t))))
			})

			s.Then("each hook runs in a distinct variable scope, so their local variables don't collide", func(t *testcase.T) {
				assert.NotEqual(t, workflow.CurrentVarScope(act(t)), workflow.CurrentVarScope(othHook.Get(t).Context(ctx.Get(t))))
			})
		})

		s.When("the context already carries a path and a variable scope, like the one where the process finishes", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				c := workflow.WithName(t.Context(), t.Random.UUID())
				return workflow.WithVarScope(c, t.Random.UUID())
			})

			s.Then("they are replaced by the declaration's location rather than nested under it", func(t *testcase.T) {
				assert.Equal(t, workflow.CurrentPath(act(t))[:len(declaredPath.Get(t))], declaredPath.Get(t))
				assert.Equal(t, len(workflow.CurrentPath(act(t))), len(declaredPath.Get(t))+1)
				assert.Equal(t, workflow.CurrentVarScope(act(t))[:len(declaredScope.Get(t))], declaredScope.Get(t))
				assert.Equal(t, len(workflow.CurrentVarScope(act(t))), len(declaredScope.Get(t))+1)
			})

			s.And("the hook was declared in the process's root variable scope", func(s *testcase.Spec) {
				declaredScope.LetValue(s, nil)

				s.Then("the hooked definition's scope is nested directly under the root", func(t *testcase.T) {
					assert.Equal(t, workflow.CurrentVarScope(act(t)), workflow.VarScope{subject.Get(t).EventID.String()})
				})
			})
		})

		s.Test("a hook registered through workflow.Defer runs where it was declared", func(t *testcase.T) {
			declCtx := workflow.Runtime{Events: eventRepository.Get(t)}.Context(t.Context())
			declCtx = wftest.ContextWithProcessID(t, declCtx, processID.Get(t))
			for _, name := range declaredPath.Get(t) {
				declCtx = workflow.WithName(declCtx, name)
			}
			for _, name := range declaredScope.Get(t) {
				declCtx = workflow.WithVarScope(declCtx, name)
			}
			assert.NoError(t, workflow.Defer(declCtx, workflow.Sequence{}))

			hooks := deferHooksOf(t, eventRepository.Get(t), processID.Get(t))
			assert.Equal(t, len(hooks), 1)
			runCtx := hooks[0].Context(t.Context())

			exp := append(workflow.Path{}, declaredPath.Get(t)...)
			exp = append(exp, hooks[0].EventID.String())
			assert.Equal(t, workflow.CurrentPath(runCtx), exp)

			expScope := append(workflow.VarScope{}, declaredScope.Get(t)...)
			expScope = append(expScope, hooks[0].EventID.String())
			assert.Equal(t, workflow.CurrentVarScope(runCtx), expScope)
		})

		s.Test("the hooked definition sees the variables of its declaration, but its own writes stay local to the hook", func(t *testcase.T) {
			vars := workflow.Vars{ProcessID: processID.Get(t), EventsRepository: eventRepository.Get(t)}

			declCtx := t.Context()
			for _, name := range declaredScope.Get(t) {
				declCtx = workflow.WithVarScope(declCtx, name)
			}
			var (
				shared = workflow.VarName(t.Random.UUID())
				local  = workflow.VarName(t.Random.UUID())
				value  = t.Random.String()
			)
			assert.NoError(t, vars.Set(declCtx, shared, value))

			runCtx := act(t)
			got, ok, err := vars.Lookup(runCtx, shared)
			assert.NoError(t, err)
			assert.True(t, ok)
			assert.Equal[any](t, got, value)

			assert.NoError(t, vars.Set(runCtx, local, t.Random.String()))
			_, ok, err = vars.Lookup(declCtx, local)
			assert.NoError(t, err)
			assert.False(t, ok, assert.MessageF("a variable first assigned by the hook must not leak into its declaration's scope"))
		})
	})
}

func deferHooksOf(tb testing.TB, repo workflow.EventRepository, pid workflow.ProcessID) []workflow.EventDefer {
	tb.Helper()
	events, err := iterkit.CollectE(repo.FindByProcessID(context.Background(), pid))
	assert.Must(tb).NoError(err)
	var hooks []workflow.EventDefer
	for _, e := range events {
		if h, ok := e.(workflow.EventDefer); ok {
			hooks = append(hooks, h)
		}
	}
	return hooks
}
