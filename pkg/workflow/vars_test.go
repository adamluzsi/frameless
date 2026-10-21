package workflow_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/ds/dscontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestVars(t *testing.T) {
	s := testcase.NewSpec(t)

	eventRepository := let.Var(s, func(t *testcase.T) workflow.EventRepository {
		return &memory.WorkflowEventRepository{}
	})

	processID := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		return mustProcessID(t)
	})

	subject := let.Var(s, func(t *testcase.T) workflow.Vars {
		return workflow.Vars{
			ProcessID:        processID.Get(t),
			EventsRepository: eventRepository.Get(t),
		}
	})

	s.Context("implements ds.Map",
		dscontract.MapE(func(tb testing.TB) ds.MapE[workflow.VarName, any] {
			return subject.Get(testcase.ToT(&tb))
		}).Spec)

	var letVarName = func(s *testcase.Spec) testcase.Var[workflow.VarName] {
		s.H().Helper()
		return let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.UUID())
		})
	}
	var letValue = func(s *testcase.Spec) testcase.Var[any] {
		s.H().Helper()
		return let.Var(s, func(t *testcase.T) any {
			return random.Pick(t.Random,
				func() any { return t.Random.Int() },
				func() any { return t.Random.String() },
				func() any { return t.Random.Bool() },
				func() any { return t.Random.Float32() },
				func() any { return t.Random.Float64() },
				func() any { return random.Slice(t.Random.IntBetween(0, 7), t.Random.UUID) },
			)()
		})
	}
	// letVarScope makes a nested variable scope, a Path made of variable scopes only.
	var letVarScope = func(s *testcase.Spec) testcase.Var[workflow.Path] {
		s.H().Helper()
		return let.Var(s, func(t *testcase.T) workflow.Path {
			return random.Slice(t.Random.IntBetween(1, 7), func() workflow.Scope {
				return workflow.Scope{Name: t.Random.UUID(), Var: true}
			})
		})
	}
	// letPath makes an execution path, one that doesn't open variable scopes
	// along the way.
	var letPath = func(s *testcase.Spec) testcase.Var[workflow.Path] {
		s.H().Helper()
		return let.Var(s, func(t *testcase.T) workflow.Path {
			return random.Slice(t.Random.IntBetween(1, 7), func() workflow.Scope {
				return workflow.Scope{Name: t.Random.UUID()}
			})
		})
	}
	var letVarScopeOther = func(s *testcase.Spec, p testcase.Var[workflow.Path]) testcase.Var[workflow.Path] {
		return let.Var(s, func(t *testcase.T) workflow.Path {
			var o = make(workflow.Path, len(p.Get(t)))
			for i, v := range p.Get(t) {
				o[i] = workflow.Scope{Name: random.Unique(t.Random.UUID, v.Name), Var: true}
			}
			random.Pick(t.Random, func() {
				if 1 < len(o) {
					slicekit.Pop(&o)
				}
			}, func() {
				o = append(o, workflow.Scope{Name: t.Random.UUID(), Var: true})
			}, func() {
				// do nothing
			})()
			return o
		})
	}

	// declareVar records an explicit declaration of a variable in a given
	// variable scope — the `var name` that brings a binding into existence,
	// independently from any declaration visible from an enclosing scope.
	var declareVar = func(t *testcase.T, name workflow.VarName, scope workflow.Path) {
		t.Helper()
		var event workflow.Event = workflow.EventDeclareVar{
			EventID:   mustEventID(t),
			ProcessID: processID.Get(t),
			Timestamp: clock.Now().UTC(),
			Path:      scope,
			Name:      name,
		}
		assert.NoError(t, eventRepository.Get(t).Create(t.Context(), &event))
	}

	// recordEvent appends an event straight into the Process event history, the
	// way an earlier execution of the Process left it behind.
	var recordEvent = func(t *testcase.T, event workflow.Event) {
		t.Helper()
		assert.NoError(t, eventRepository.Get(t).Create(t.Context(), &event))
	}

	s.Describe("latest-state queries at an earlier execution path", func(s *testcase.Spec) {
		ctx := let.Var(s, func(t *testcase.T) context.Context {
			return workflow.WithName(workflow.WithName(t.Context(), "sequence"), "[2]")
		})
		name := letVarName(s)
		act := func(t *testcase.T) map[workflow.VarName]any {
			vars := subject.Get(t)
			values, err := vars.ToMap(ctx.Get(t))
			assert.NoError(t, err)
			value, found, err := vars.Lookup(ctx.Get(t), name.Get(t))
			assert.NoError(t, err)
			expected, exists := values[name.Get(t)]
			assert.Equal(t, found, exists)
			assert.Equal(t, value, expected)
			value, err = vars.Get(ctx.Get(t), name.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, value, expected)
			bindings := map[workflow.VarName]any{}
			for binding, err := range vars.All(ctx.Get(t)) {
				assert.NoError(t, err)
				bindings[binding.Name] = binding.Value
			}
			assert.Equal(t, bindings, values)
			keys := map[workflow.VarName]bool{}
			for key, err := range vars.Keys(ctx.Get(t)) {
				assert.NoError(t, err)
				keys[key] = true
			}
			assert.Equal(t, len(keys), len(values))
			for key := range values {
				assert.True(t, keys[key])
			}
			return values
		}

		s.Then("sees later assignments and deletions without treating paths as a timeline", func(t *testcase.T) {
			vars := subject.Get(t)
			assert.NoError(t, vars.Set(ctx.Get(t), name.Get(t), true))
			assert.Equal[any](t, act(t)[name.Get(t)], true)

			// [10] follows [2] in execution, but precedes it lexically.
			later := workflow.WithName(workflow.WithName(t.Context(), "sequence"), "[10]")
			assert.NoError(t, vars.Set(later, name.Get(t), false))
			assert.Equal[any](t, act(t)[name.Get(t)], false)
			assert.NoError(t, vars.Delete(later, name.Get(t)))
			assert.Empty(t, act(t))
		})

		s.When("the reader is in a nested variable scope", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				return workflow.WithVarScope(ctx.Super(t), "nested")
			})

			s.Then("sees current visible bindings while excluding sibling bindings", func(t *testcase.T) {
				vars := subject.Get(t)
				assert.NoError(t, vars.Set(t.Context(), name.Get(t), true))
				assert.Equal[any](t, act(t)[name.Get(t)], true)
				assert.NoError(t, vars.Set(workflow.WithName(t.Context(), "later"), name.Get(t), false))
				assert.NoError(t, vars.Set(workflow.WithVarScope(t.Context(), "sibling"), "hidden", true))
				assert.Equal(t, act(t), map[workflow.VarName]any{name.Get(t): false})
			})
		})
	})

	s.Describe("#Set", func(s *testcase.Spec) {
		var (
			ctx   = let.Context(s)
			name  = letVarName(s)
			value = letValue(s)
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Set(ctx.Get(t), name.Get(t), value.Get(t))
		}

		s.Then("the call returns no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Then("an EventSetVar is appended to the process events with the assigned value", func(t *testcase.T) {
			assert.NoError(t, act(t))

			events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

			assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
				ve, ok := e.(workflow.EventSetVar)
				assert.True(tb, ok)
				assert.Equal(tb, ve.Name, name.Get(t))
				assert.Equal[any](tb, ve.Value, value.Get(t))
				assert.NotEmpty(tb, ve.ProcessID)
				assert.NotEmpty(tb, ve.Timestamp)
			})
		})

		s.When("an execution path is present in the context", func(s *testcase.Spec) {
			execPath := letPath(s)

			ctx.Let(s, func(t *testcase.T) context.Context {
				return withPath(ctx.Super(t), execPath.Get(t))
			})

			s.Then("the EventSetVar remembers the execution path it was made at", func(t *testcase.T) {
				assert.NoError(t, act(t))

				events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

				assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
					ve, ok := e.(workflow.EventSetVar)
					assert.True(tb, ok)
					assert.Equal(tb, ve.Name, name.Get(t))
					assertSamePath(tb, execPath.Get(t), ve.Path)
				})
			})
		})

		s.When("variable scope is set in the context", func(s *testcase.Spec) {
			varScope := letVarScope(s)

			ctx.Let(s, func(t *testcase.T) context.Context {
				return withVarScope(ctx.Super(t), varScope.Get(t))
			})

			s.Then("an EventSetVar is appended to the process events with the assigned value", func(t *testcase.T) {
				assert.NoError(t, act(t))

				events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

				assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
					ve, ok := e.(workflow.EventSetVar)
					assert.True(tb, ok)
					assert.Equal(tb, ve.Name, name.Get(t))
					assert.Equal[any](tb, ve.Value, value.Get(t))
					assert.NotEmpty(tb, ve.ProcessID)
					assert.NotEmpty(tb, ve.Timestamp)
				})
			})

			s.Then("the variable is declared against the current variable scope", func(t *testcase.T) {
				assert.NoError(t, act(t))

				events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

				assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
					ve, ok := e.(workflow.EventDeclareVar)
					assert.True(tb, ok)
					assert.Equal(tb, ve.Name, name.Get(t))
					assertSamePath(tb, varScope.Get(t), ve.Path.VarScope())
				})
			})

			s.Then("the EventSetVar names the variable scope of the variable it assigned", func(t *testcase.T) {
				assert.NoError(t, act(t))

				events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

				assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
					ve, ok := e.(workflow.EventSetVar)
					assert.True(tb, ok)
					assert.Equal(tb, ve.Name, name.Get(t))
					assertSamePath(tb, varScope.Get(t), ve.Path.VarScope())
				})
			})

			s.And("if the variable was already assigned in a previous outer-scope", func(s *testcase.Spec) {
				outerScope := let.Var(s, func(t *testcase.T) workflow.Path {
					scope := varScope.Get(t)[0 : len(varScope.Get(t))-1]
					assert.NotEqual(t, scope, varScope.Get(t))
					return scope
				})

				s.Before(func(t *testcase.T) {
					assert.NoError(t, subject.Get(t).Set(withVarScope(t.Context(), outerScope.Get(t)), name.Get(t), t.Random.UUID()))
				})

				s.Then("the variable value is assigned to the latest value, without re-declaring the scope", func(t *testcase.T) {
					assert.NoError(t, act(t))

					events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

					assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
						ve, ok := e.(workflow.EventSetVar)
						assert.True(tb, ok)
						assert.Equal(tb, ve.Name, name.Get(t))
						assert.Equal[any](tb, ve.Value, value.Get(t))
					})

					assert.NoneOf(t, events, func(tb testing.TB, e workflow.Event) {
						ve, ok := e.(workflow.EventDeclareVar)
						assert.True(tb, ok)
						assert.Equal(tb, ve.Name, name.Get(t))
						assertSamePath(tb, varScope.Get(t), ve.Path.VarScope(),
							assert.MessageF("the variable was already declared in the enclosing scope, so no inner re-declaration is expected"))
					})
				})

				s.Then("the EventSetVar names the outer variable scope, where the assigned variable lives", func(t *testcase.T) {
					assert.NoError(t, act(t))

					events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

					assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
						ve, ok := e.(workflow.EventSetVar)
						assert.True(tb, ok)
						assert.Equal(tb, ve.Name, name.Get(t))
						assert.Equal[any](tb, ve.Value, value.Get(t))
						// the outer scope is the root scope when the nested one is a single level deep
						assertSamePath(tb, outerScope.Get(t), ve.Path.VarScope())
					})
				})

				s.Then("the EventSetVar still records the location the assignment was made at", func(t *testcase.T) {
					assert.NoError(t, act(t))

					events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

					assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
						ve, ok := e.(workflow.EventSetVar)
						assert.True(tb, ok)
						assert.Equal[any](tb, ve.Value, value.Get(t))
						assert.True(tb, ve.Path.Equal(varScope.Get(t)),
							assert.MessageF("the assignment was made at %v, but recorded at %v", varScope.Get(t), ve.Path))
					})
				})
			})
		})
	})

	s.Describe("#Get / #Lookup", func(s *testcase.Spec) {
		var (
			readContext = let.Context(s)
			name        = letVarName(s)
		)
		actGet := let.Act2(func(t *testcase.T) (any, error) {
			return subject.Get(t).Get(readContext.Get(t), name.Get(t))
		})
		actLookup := let.Act3(func(t *testcase.T) (any, bool, error) {
			return subject.Get(t).Lookup(readContext.Get(t), name.Get(t))
		})

		s.When("variable by this name is already assigned", func(s *testcase.Spec) {
			writeContext := let.Context(s)
			value := letValue(s)

			s.Before(func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Set(writeContext.Get(t), name.Get(t), value.Get(t)))
			})

			s.Then("value can be retrieved with #Get", func(t *testcase.T) {
				got, err := actGet(t)
				assert.NoError(t, err)
				assert.Equal(t, value.Get(t), got)
			})

			s.Then("value can be retrieved with #Lookup", func(t *testcase.T) {
				got, ok, err := actLookup(t)
				assert.NoError(t, err)
				assert.True(t, ok)
				assert.Equal(t, value.Get(t), got)
			})

			s.And("the variable scopes", func(s *testcase.Spec) {
				var (
					readScope  = let.Var[workflow.Path](s, nil)
					writeScope = let.Var[workflow.Path](s, nil)
				)
				readContext.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(readContext.Super(t), readScope.Get(t))
				})
				writeContext.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(writeContext.Super(t), writeScope.Get(t))
				})

				s.Context("are the same", func(s *testcase.Spec) {
					commonScope := letVarScope(s)
					readScope.Let(s, commonScope.Get)
					writeScope.Let(s, commonScope.Get)

					s.Then("#Get will find the value", func(t *testcase.T) {
						got, err := actGet(t)
						assert.NoError(t, err)
						assert.Equal(t, value.Get(t), got)
					})

					s.Then("#Lookup will find the value", func(t *testcase.T) {
						got, ok, err := actLookup(t)
						assert.NoError(t, err)
						assert.True(t, ok)
						assert.Equal(t, value.Get(t), got)
					})
				})

				s.Context("matched by prefix, the reading context is a sub var scope of writing", func(s *testcase.Spec) {
					writeScope.Let(s, letVarScope(s).Get)
					readScope.Let(s, func(t *testcase.T) workflow.Path {
						o := slicekit.Clone(writeScope.Get(t))
						t.Random.Repeat(1, 3, func() {
							o = append(o, workflow.Scope{Name: t.Random.UUID(), Var: true})
						})
						return o
					})

					s.Then("#Get will find the value", func(t *testcase.T) {
						got, err := actGet(t)
						assert.NoError(t, err)
						assert.Equal(t, value.Get(t), got)
					})

					s.Then("#Lookup will find the value", func(t *testcase.T) {
						got, ok, err := actLookup(t)
						assert.NoError(t, err)
						assert.True(t, ok)
						assert.Equal(t, value.Get(t), got)
					})
				})

				s.Context("read var scope is outer than write scope", func(s *testcase.Spec) {
					readScope.Let(s, letVarScope(s).Get)
					writeScope.Let(s, func(t *testcase.T) workflow.Path {
						o := slicekit.Clone(readScope.Get(t))
						t.Random.Repeat(1, 3, func() {
							o = append(o, workflow.Scope{Name: t.Random.UUID(), Var: true})
						})
						return o
					})

					s.Then("#Get will NOT find the value because it is not part of its var-scope", func(t *testcase.T) {
						got, err := actGet(t)
						assert.NoError(t, err)
						assert.Nil(t, got)
					})

					s.Then("#Lookup will NOT find the value because it is not part of its var-scope", func(t *testcase.T) {
						got, ok, err := actLookup(t)
						assert.NoError(t, err)
						assert.False(t, ok)
						assert.Nil(t, got)
					})
				})

				s.Context("scopes are not empty and completely different", func(s *testcase.Spec) {
					readScope.Let(s, letVarScope(s).Get)
					writeScope.Let(s, letVarScopeOther(s, readScope).Get)

					s.Then("#Get will NOT find the value because it is in a different var-scope", func(t *testcase.T) {
						got, err := actGet(t)
						assert.NoError(t, err)
						assert.Nil(t, got)
					})

					s.Then("#Lookup will NOT find the value because it is in a different var-scope", func(t *testcase.T) {
						got, ok, err := actLookup(t)
						assert.NoError(t, err)
						assert.False(t, ok)
						assert.Nil(t, got)
					})
				})
			})
		})
	})

	//──────────────────────────────────────────────────────────────────────
	// #Delete

	s.Describe("#Delete", func(s *testcase.Spec) {
		var (
			ctx  = let.Context(s)
			name = letVarName(s)
		)
		act := func(t *testcase.T) error {
			return subject.Get(t).Delete(ctx.Get(t), name.Get(t))
		}

		s.Then("the call returns no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Context("the variable has an assigned value", func(s *testcase.Spec) {
			value := letValue(s)

			setContext := let.Context(s)

			s.Before(func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Set(setContext.Get(t), name.Get(t), value.Get(t)))
			})

			s.Then("#Lookup will report the key as no longer found", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok, err := subject.Get(t).Lookup(ctx.Get(t), name.Get(t))
				assert.NoError(t, err)
				assert.False(t, ok)
				assert.Nil(t, got)
			})

			s.And("an execution path is present in the context", func(s *testcase.Spec) {
				execPath := letPath(s)

				ctx.Let(s, func(t *testcase.T) context.Context {
					return withPath(ctx.Super(t), execPath.Get(t))
				})

				s.Then("the EventDeleteVar remembers the execution path it was made at", func(t *testcase.T) {
					assert.NoError(t, act(t))

					events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

					assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
						ve, ok := e.(workflow.EventDeleteVar)
						assert.True(tb, ok)
						assert.Equal(tb, ve.Name, name.Get(t))
						assert.Equal(tb, ve.Path, execPath.Get(t))
					})
				})
			})

			s.And("the variable scope is set in the context", func(s *testcase.Spec) {
				varScope := letVarScope(s)

				ctx.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(ctx.Super(t), varScope.Get(t))
				})

				s.Then("#Lookup will report the key as no longer found from a parent scope", func(t *testcase.T) {
					assert.NoError(t, act(t))

					readCtx := withVarScope(t.Context(), varScope.Get(t)[:0])
					got, ok, err := subject.Get(t).Lookup(readCtx, name.Get(t))
					assert.NoError(t, err)
					assert.False(t, ok)
					assert.Nil(t, got)
				})
			})

			s.And("the variable lives in a variable scope enclosing the one the deletion is made from", func(s *testcase.Spec) {
				varScope := letVarScope(s)

				setContext.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(setContext.Super(t), varScope.Get(t))
				})
				ctx.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(ctx.Super(t), slicekit.Merge(varScope.Get(t), varScopeOf(t.Random.UUID())))
				})

				s.Then("the EventDeleteVar names the variable scope of the deleted variable", func(t *testcase.T) {
					assert.NoError(t, act(t))

					events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

					assert.OneOf(t, events, func(tb testing.TB, e workflow.Event) {
						ve, ok := e.(workflow.EventDeleteVar)
						assert.True(tb, ok)
						assert.Equal(tb, ve.Name, name.Get(t))
						assertSamePath(tb, varScope.Get(t), ve.Path.VarScope())
					})
				})

				s.Then("the variable is deleted from its own variable scope", func(t *testcase.T) {
					assert.NoError(t, act(t))

					_, ok, err := subject.Get(t).Lookup(setContext.Get(t), name.Get(t))
					assert.NoError(t, err)
					assert.False(t, ok)
				})
			})

			s.Context("but the deletion is issued from a variable scope which doesn't have access to the created variable", func(s *testcase.Spec) {
				setContext.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(setContext.PreviousValue(t), varScopeOf("foo", "bar", "baz"))
				})
				ctx.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(setContext.PreviousValue(t), varScopeOf("foo", "bar"))
				})

				s.Then("no error occurs", func(t *testcase.T) {
					assert.NoError(t, act(t))
				})

				s.Then("an EventDeleteVar is NOT emitted to the process events", func(t *testcase.T) {
					assert.NoError(t, act(t))

					events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

					assert.NoneOf(t, events, func(tb testing.TB, e workflow.Event) {
						_, ok := e.(workflow.EventDeleteVar)
						assert.True(tb, ok)
					})
				})

				s.Then("the variable in the inner var-scope is NOT deleted", func(t *testcase.T) {
					assert.NoError(t, act(t))

					got, ok, err := subject.Get(t).Lookup(setContext.Get(t), name.Get(t))
					assert.NoError(t, err)
					assert.True(t, ok)
					assert.Equal(t, got, value.Get(t))
				})
			})
		})

		s.Context("the variable is not assigned", func(s *testcase.Spec) {
			s.Then("the call still returns no error", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("an EventDeleteVar is NOT emitted to the process events", func(t *testcase.T) {
				assert.NoError(t, act(t))

				events := getProcessEvents(t, eventRepository.Get(t), processID.Get(t))

				assert.NoneOf(t, events, func(tb testing.TB, e workflow.Event) {
					_, ok := e.(workflow.EventDeleteVar)
					assert.True(tb, ok)
				})
			})
		})

		s.Context("another variable with a different name is assigned", func(s *testcase.Spec) {
			otherName := letVarName(s)
			otherValue := letValue(s)

			s.Before(func(t *testcase.T) {
				assert.NoError(t, subject.Get(t).Set(ctx.Get(t), otherName.Get(t), otherValue.Get(t)))
			})

			s.Then("the other variable remains retrievable after the delete", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok, err := subject.Get(t).Lookup(ctx.Get(t), otherName.Get(t))
				assert.NoError(t, err)
				assert.True(t, ok)
				assert.Equal(t, otherValue.Get(t), got)
			})
		})

		s.Context("a delete event is recorded after the variable was declared in a sub scope", func(s *testcase.Spec) {
			declareScope := letVarScope(s)
			value := letValue(s)

			s.Before(func(t *testcase.T) {
				base := clock.Now().UTC()

				declareEventID, err := workflow.MakeEventID()
				assert.NoError(t, err)
				declareEvent := workflow.Event(workflow.EventDeclareVar{
					EventID:   declareEventID,
					ProcessID: processID.Get(t),
					Timestamp: base,
					Name:      name.Get(t),
					Path:      declareScope.Get(t),
				})
				assert.NoError(t, eventRepository.Get(t).Create(t.Context(), &declareEvent))

				setEventID, err := workflow.MakeEventID()
				assert.NoError(t, err)
				setEvent := workflow.Event(workflow.EventSetVar{
					EventID:   setEventID,
					ProcessID: processID.Get(t),
					Timestamp: base.Add(time.Nanosecond),
					Name:      name.Get(t),
					Path:      declareScope.Get(t),
					Value:     value.Get(t),
				})
				assert.NoError(t, eventRepository.Get(t).Create(t.Context(), &setEvent))

				deleteEventID, err := workflow.MakeEventID()
				assert.NoError(t, err)
				deleteEvent := workflow.Event(workflow.EventDeleteVar{
					EventID:   deleteEventID,
					ProcessID: processID.Get(t),
					Timestamp: base.Add(time.Second),
					Name:      name.Get(t),
					Path:      declareScope.Get(t),
				})
				assert.NoError(t, eventRepository.Get(t).Create(t.Context(), &deleteEvent))
			})

			s.Then("the deletion applies where the declaration is visible", func(t *testcase.T) {
				readCtx := withVarScope(t.Context(), declareScope.Get(t))
				_, ok, err := subject.Get(t).Lookup(readCtx, name.Get(t))
				assert.NoError(t, err)
				assert.False(t, ok)
			})
		})
	})

	s.Describe("a variable declared in an outer scope", func(s *testcase.Spec) {
		var (
			name       = letVarName(s)
			outerValue = letValue(s)
			innerValue = letValue(s)

			outerScope = let.Var(s, func(t *testcase.T) workflow.Path {
				return varScopeOf("foo", "bar")
			})
			innerScope = let.Var(s, func(t *testcase.T) workflow.Path {
				return slicekit.Merge(outerScope.Get(t), varScopeOf("baz"))
			})
		)

		// read returns the variable under test as it is seen from a given
		// variable scope.
		//
		// Vars folds the very same event history through two read paths — the
		// single-variable #Lookup and the whole-scope #ToMap — so every scenario
		// below is asserted through both, to keep the two from drifting apart on
		// what a declaration makes visible.
		var read = func(t *testcase.T, scope workflow.Path) (any, bool) {
			t.Helper()
			var readCtx = withVarScope(t.Context(), scope)

			got, ok, err := subject.Get(t).Lookup(readCtx, name.Get(t))
			assert.NoError(t, err)

			vs, err := subject.Get(t).ToMap(readCtx)
			assert.NoError(t, err)
			mapGot, mapOK := vs[name.Get(t)]

			assert.Equal(t, ok, mapOK,
				assert.MessageF("#Lookup and #ToMap disagree on whether the variable is visible"))
			assert.Equal[any](t, got, mapGot,
				assert.MessageF("#Lookup and #ToMap disagree on the value of the variable"))

			return got, ok
		}

		s.Before(func(t *testcase.T) {
			// var x = outerValue, declared in the outer scope
			assert.NoError(t, subject.Get(t).Set(withVarScope(t.Context(), outerScope.Get(t)), name.Get(t), outerValue.Get(t)))
		})

		s.When("the variable is assigned from a nested scope", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				// x = innerValue — a Set, not a re-declaration
				assert.NoError(t, subject.Get(t).Set(withVarScope(t.Context(), innerScope.Get(t)), name.Get(t), innerValue.Get(t)))
			})

			s.Then("the assignment affects the outer scope's binding", func(t *testcase.T) {
				got, ok := read(t, outerScope.Get(t))
				assert.True(t, ok)
				assert.Equal[any](t, got, innerValue.Get(t),
					assert.MessageF("a Set writes to the nearest visible declaration, like Go's = operator"))
			})

			s.Then("the nested scope sees the same shared value", func(t *testcase.T) {
				got, ok := read(t, innerScope.Get(t))
				assert.True(t, ok)
				assert.Equal[any](t, got, innerValue.Get(t))
			})
		})

		s.When("the variable is explicitly re-declared in a nested scope", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				// x := innerValue — a fresh binding in the inner scope, shadowing the outer
				declareVar(t, name.Get(t), innerScope.Get(t))
				assert.NoError(t, subject.Get(t).Set(withVarScope(t.Context(), innerScope.Get(t)), name.Get(t), innerValue.Get(t)))
			})

			s.Then("the nested scope sees the shadowing value", func(t *testcase.T) {
				got, ok := read(t, innerScope.Get(t))
				assert.True(t, ok)
				assert.Equal[any](t, got, innerValue.Get(t))
			})

			s.Then("the outer scope still sees its own value", func(t *testcase.T) {
				got, ok := read(t, outerScope.Get(t))
				assert.True(t, ok, "an explicit re-declaration isolates the inner binding, like Go's := operator")
				assert.Equal[any](t, got, outerValue.Get(t))
			})

			s.When("the shadowing variable is deleted from the nested scope", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					assert.NoError(t, subject.Get(t).Delete(withVarScope(t.Context(), innerScope.Get(t)), name.Get(t)))
				})

				s.Then("the nested scope no longer sees the variable", func(t *testcase.T) {
					got, ok := read(t, innerScope.Get(t))
					assert.False(t, ok,
						assert.MessageF("a delete removes the innermost binding the scope reads through, it does not fall back to the shadowed one"))
					assert.Nil(t, got)
				})

				s.Then("the outer scope binding is untouched", func(t *testcase.T) {
					got, ok := read(t, outerScope.Get(t))
					assert.True(t, ok)
					assert.Equal[any](t, got, outerValue.Get(t))
				})
			})

			// x := outerValue
			// { x := innerValue }
			// x = laterValue
			//
			// The nested re-declaration is a variable of its own, so it must not
			// take the name over from the outer scope: an assignment made from the
			// outer scope afterwards still belongs to the outer variable.
			s.And("the outer scope assigns to the variable after the nested scope re-declared it", func(s *testcase.Spec) {
				laterValue := letValue(s)

				s.Before(func(t *testcase.T) {
					assert.NoError(t, subject.Get(t).Set(withVarScope(t.Context(), outerScope.Get(t)), name.Get(t), laterValue.Get(t)))
				})

				s.Then("the outer scope sees the assigned value", func(t *testcase.T) {
					got, ok := read(t, outerScope.Get(t))
					assert.True(t, ok)
					assert.Equal[any](t, got, laterValue.Get(t),
						assert.MessageF("the assignment is made to the outer variable, the one visible from the outer scope"))
				})

				s.Then("the nested scope still sees its own shadowing value", func(t *testcase.T) {
					got, ok := read(t, innerScope.Get(t))
					assert.True(t, ok)
					assert.Equal[any](t, got, innerValue.Get(t),
						assert.MessageF("the shadowing variable is a different variable, which the outer assignment doesn't reach"))
				})
			})

			s.And("the variable is deleted from the outer scope after the nested scope re-declared it", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					assert.NoError(t, subject.Get(t).Delete(withVarScope(t.Context(), outerScope.Get(t)), name.Get(t)))
				})

				s.Then("the outer scope no longer sees the variable", func(t *testcase.T) {
					got, ok := read(t, outerScope.Get(t))
					assert.False(t, ok,
						assert.MessageF("the delete removes the outer variable, the one visible from the outer scope"))
					assert.Nil(t, got)
				})

				s.Then("the nested scope still sees its own shadowing value", func(t *testcase.T) {
					got, ok := read(t, innerScope.Get(t))
					assert.True(t, ok)
					assert.Equal[any](t, got, innerValue.Get(t),
						assert.MessageF("the shadowing variable is a different variable, which the outer delete doesn't reach"))
				})
			})

			// A recorded assignment changes the variable of its Name that lives in
			// its Scope. It's the event history alone that decides this, so a
			// Process resumed from its history sees the very same variables.
			s.And("the history holds an assignment to the variable of the outer scope, recorded after the re-declaration", func(s *testcase.Spec) {
				recordedValue := letValue(s)

				s.Before(func(t *testcase.T) {
					recordEvent(t, workflow.EventSetVar{
						EventID:   mustEventID(t),
						ProcessID: processID.Get(t),
						Timestamp: clock.Now().UTC(),
						Name:      name.Get(t),
						Path:      outerScope.Get(t),
						Value:     recordedValue.Get(t),
					})
				})

				s.Then("the outer scope sees the recorded value", func(t *testcase.T) {
					got, ok := read(t, outerScope.Get(t))
					assert.True(t, ok)
					assert.Equal[any](t, got, recordedValue.Get(t))
				})

				s.Then("the nested scope still sees its own shadowing value", func(t *testcase.T) {
					got, ok := read(t, innerScope.Get(t))
					assert.True(t, ok)
					assert.Equal[any](t, got, innerValue.Get(t),
						assert.MessageF("the assignment names the outer scope, so it can't reach the variable of the nested one"))
				})
			})

			s.And("the history holds a deletion of the variable of the outer scope, recorded after the re-declaration", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					recordEvent(t, workflow.EventDeleteVar{
						EventID:   mustEventID(t),
						ProcessID: processID.Get(t),
						Timestamp: clock.Now().UTC(),
						Name:      name.Get(t),
						Path:      outerScope.Get(t),
					})
				})

				s.Then("the outer scope no longer sees the variable", func(t *testcase.T) {
					_, ok := read(t, outerScope.Get(t))
					assert.False(t, ok)
				})

				s.Then("the nested scope still sees its own shadowing value", func(t *testcase.T) {
					got, ok := read(t, innerScope.Get(t))
					assert.True(t, ok)
					assert.Equal[any](t, got, innerValue.Get(t),
						assert.MessageF("the deletion names the outer scope, so it can't reach the variable of the nested one"))
				})
			})
		})

		s.When("the variable is re-declared in the scope it already lives in", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				// var x — a fresh declaration over the binding that is already there
				declareVar(t, name.Get(t), outerScope.Get(t))
			})

			s.Then("the binding is reset to declared but unassigned", func(t *testcase.T) {
				got, ok := read(t, outerScope.Get(t))
				assert.True(t, ok, "the declaration keeps the variable in existence")
				assert.Nil(t, got, "a declaration is `var x`, it does not carry the previous value over")
			})
		})
	})

	// A variable scope is a segment of the Path, so its identity is its location,
	// the way a block of a Go function is the block at its place in the source:
	//
	//	for _, x := range xs {
	//		{ v := x }      // [0]/block
	//		{ see(v) }      // [1]/block, v is undefined here
	//	}
	s.Describe("variable scopes of the same name, at different locations", func(s *testcase.Spec) {
		var (
			name      = letVarName(s)
			value     = letValue(s)
			scopeName = let.String(s)
			written   = let.Var(s, func(t *testcase.T) context.Context {
				return workflow.WithVarScope(workflow.WithName(t.Context(), "[0]"), scopeName.Get(t))
			})
			readFrom = let.Var(s, func(t *testcase.T) context.Context {
				return workflow.WithVarScope(workflow.WithName(t.Context(), "[1]"), scopeName.Get(t))
			})
			// declaredAt [optional] is where the variable is already declared, before the write.
			declaredAt = let.VarOf[context.Context](s, nil)
		)
		act := func(t *testcase.T) (any, bool) {
			got, ok, err := subject.Get(t).Lookup(readFrom.Get(t), name.Get(t))
			assert.NoError(t, err)
			return got, ok
		}
		s.Before(func(t *testcase.T) {
			if ctx := declaredAt.Get(t); ctx != nil {
				assert.NoError(t, subject.Get(t).Set(ctx, name.Get(t), t.Random.String()))
			}
			assert.NoError(t, subject.Get(t).Set(written.Get(t), name.Get(t), value.Get(t)))
		})

		s.Then("the variables of one are not visible from the other", func(t *testcase.T) {
			_, ok := act(t)
			assert.False(t, ok, "sibling variable scopes must not share variables just because their names match")
		})

		s.When("the variable scope is read at the very same location, like a replay of the same block", func(s *testcase.Spec) {
			readFrom.Let(s, func(t *testcase.T) context.Context {
				return workflow.WithVarScope(workflow.WithName(t.Context(), "[0]"), scopeName.Get(t))
			})

			s.Then("it sees the variables of the scope again", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)
				assert.Equal(t, got, value.Get(t))
			})
		})

		s.When("the variable is declared in a variable scope enclosing both", func(s *testcase.Spec) {
			enclosing := let.Var(s, func(t *testcase.T) context.Context {
				return workflow.WithVarScope(t.Context(), "enclosing")
			})
			written.Let(s, func(t *testcase.T) context.Context {
				return workflow.WithVarScope(workflow.WithName(enclosing.Get(t), "[0]"), scopeName.Get(t))
			})
			readFrom.Let(s, func(t *testcase.T) context.Context {
				return workflow.WithVarScope(workflow.WithName(enclosing.Get(t), "[1]"), scopeName.Get(t))
			})
			declaredAt.Let(s, enclosing.Get)

			s.Then("the assignment of one writes through, and the other sees it", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)
				assert.Equal(t, got, value.Get(t))
			})
		})
	})

	s.Test("the variable scope (not the current execution path) determines the visibility", func(t *testcase.T) {
		var (
			varScopeName = t.Random.UUID()
			varName      = workflow.VarName(t.Random.UUID())
			varValue     = t.Random.String()
		)

		participants := workflow.Participants{
			"set_var": func(ctx context.Context) error {
				vars, err := workflow.GetVars(ctx)
				if err != nil {
					return err
				}
				return vars.Set(ctx, varName, varValue)
			},
		}

		def := workflow.Sequence{
			&workflow.Execute{Participant: "set_var"},
		}

		r := workflow.Runtime{
			Participants: participants,
			Events:       &memory.WorkflowEventRepository{},
			Locks:        &memory.WorkflowProcessLocks{},
		}

		p := mustProcessID(t)
		scopedCtx := workflow.WithVarScope(t.Context(), varScopeName)

		assert.NoError(t, r.Bind(scopedCtx, p, def))
		assert.NoError(t, r.Execute(scopedCtx, p))

		vs := workflow.Vars{ProcessID: p, EventsRepository: r.Events}

		t.Log("#Get can find it")
		gotI, err := vs.Get(scopedCtx, varName)
		assert.NoError(t, err)
		got, ok := gotI.(string)
		assert.True(t, ok)
		assert.Equal(t, got, varValue)

		t.Log("#Lookup can find it")
		gotI, ok, err = vs.Lookup(scopedCtx, varName)
		assert.NoError(t, err)
		got, ok = gotI.(string)
		assert.True(t, ok)
		assert.Equal(t, got, varValue)

		t.Log("value is present in #ToMap")
		m, err := vs.ToMap(scopedCtx)
		assert.NoError(t, err)
		assert.Equal[any](t, varValue, m[varName])
	})
}

func TestDeclareVar(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		name = let.As[workflow.VarName](let.String(s))
	)
	subject := let.Var(s, func(t *testcase.T) workflow.DeclareVar {
		return workflow.DeclareVar{Name: name.Get(t)}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			Context = let.Context(s)
			// declarationVarScope is the variable scope that the declaration
			// step happens to run under. It defaults to the root scope.
			declarationVarScope = let.VarOf[workflow.Path](s, nil)
			processID           = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			ctx := withVarScope(Context.Get(t), declarationVarScope.Get(t))
			return c.ActExecuteDefinition(t, ctx, processID.Get(t), subject.Get(t))
		})

		vars := LetVars(s, c.Runtime, processID)

		// declarationEvents collects every declaration recorded for the variable
		// under test, so that assertions can talk about which variable scope a
		// declaration landed in, and about how many of them there are.
		var declarationEvents = func(t *testcase.T) []workflow.EventDeclareVar {
			t.Helper()
			var out []workflow.EventDeclareVar
			for _, e := range mustHistory(t, c.Runtime.Get(t), processID.Get(t)) {
				de, ok := e.(workflow.EventDeclareVar)
				if !ok || de.Name != name.Get(t) {
					continue
				}
				out = append(out, de)
			}
			return out
		}

		// lookup reads the variable under test as it is seen from a given variable scope.
		var lookup = func(t *testcase.T, scope workflow.Path) (any, bool) {
			t.Helper()
			got, ok, err := vars.Get(t).Lookup(withVarScope(t.Context(), scope), name.Get(t))
			assert.NoError(t, err)
			return got, ok
		}

		var ThenExecutionIsIdempotent = func(s *testcase.Spec) {
			s.H().Helper()
			s.Then("execution is idempotent", func(t *testcase.T) {
				assert.NoError(t, act(t)) // first pass

				firstPassEvents := mustHistory(t, c.Runtime.Get(t), processID.Get(t))

				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))

					assert.Equal(t, mustHistory(t, c.Runtime.Get(t), processID.Get(t)), firstPassEvents)
				})
			})
		}

		s.Then("the variable comes into existence, holding no value", func(t *testcase.T) {
			assert.NoError(t, act(t))

			got, ok := lookup(t, declarationVarScope.Get(t))
			assert.True(t, ok, "the declared variable is expected to be found")
			assert.Nil(t, got, "a declaration is the `var name` half of a variable, it assigns no value")
		})

		s.Then("exactly one EventDeclareVar is recorded, remembering where the step ran", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.OneOf(t, declarationEvents(t), func(tb testing.TB, e workflow.EventDeclareVar) {
				assert.Equal(tb, e.Name, name.Get(t))
				assert.Equal(tb, e.Path.VarScope(), declarationVarScope.Get(t))
				assert.NotEmpty(tb, e.Path, "the execution path of the declaration step is expected")
			})
			assert.Equal(t, len(declarationEvents(t)), 1,
				"a single declaration step is expected to declare the variable once")
		})

		ThenExecutionIsIdempotent(s)

		s.Then("replaying the step after a later step assigned the variable will not erase the value", func(t *testcase.T) {
			assert.NoError(t, act(t))

			// a later workflow step assigns a value to the declared variable
			var value = t.Random.UUID()
			setVar(t, c.Runtime.Get(t), processID.Get(t), name.Get(t), value)

			assert.NoError(t, act(t)) // the very same step gets replayed

			got, ok := lookup(t, declarationVarScope.Get(t))
			assert.True(t, ok)
			assert.Equal[any](t, got, value,
				"the replayed declaration step must not undo what the steps after it did")
		})

		s.When("the step runs in a nested variable scope", func(s *testcase.Spec) {
			declarationVarScope.Let(s, func(t *testcase.T) workflow.Path {
				return varScopeOf(t.Random.UUID(), t.Random.UUID())
			})

			s.Then("the declaration is recorded in that variable scope", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.OneOf(t, declarationEvents(t), func(tb testing.TB, e workflow.EventDeclareVar) {
					assert.Equal(tb, e.Path.VarScope(), declarationVarScope.Get(t))
				})
			})

			s.Then("the variable is not visible from an unrelated variable scope", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok := lookup(t, varScopeOf(t.Random.UUID()))
				assert.False(t, ok, "a scoped declaration must not leak out of its own variable scope")
			})

			ThenExecutionIsIdempotent(s)
		})

		s.When("the variable is already declared and assigned in an enclosing variable scope", func(s *testcase.Spec) {
			var (
				enclosingVarScope = let.Var(s, func(t *testcase.T) workflow.Path {
					return varScopeOf(t.Random.UUID())
				})
				enclosingValue = wftest.LetValue(s)
			)

			declarationVarScope.Let(s, func(t *testcase.T) workflow.Path {
				return slicekit.Merge(enclosingVarScope.Get(t), varScopeOf(t.Random.UUID()))
			})

			s.Before(func(t *testcase.T) {
				ctx := withVarScope(t.Context(), enclosingVarScope.Get(t))
				assert.NoError(t, vars.Get(t).Set(ctx, name.Get(t), enclosingValue.Get(t)))
			})

			s.Then("the declaration shadows it with a fresh binding that holds no value", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok := lookup(t, declarationVarScope.Get(t))
				assert.True(t, ok)
				assert.Nil(t, got, "a shadowing declaration is a fresh `var name`, it carries no value over")
			})

			s.Then("the binding of the enclosing variable scope keeps its value", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok := lookup(t, enclosingVarScope.Get(t))
				assert.True(t, ok)
				assert.Equal[any](t, got, enclosingValue.Get(t),
					"shadowing must not reach into the enclosing variable scope")
			})

			ThenExecutionIsIdempotent(s)
		})

		s.When("Global is set", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.DeclareVar {
				d := subject.Super(t)
				d.Global = true
				return d
			})

			// the step runs under a nested variable scope,
			// which a Global declaration is expected to escape.
			declarationVarScope.Let(s, func(t *testcase.T) workflow.Path {
				return varScopeOf(t.Random.UUID(), t.Random.UUID())
			})

			s.Then("the variable is declared in the root scope, not the one the step ran under", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.OneOf(t, declarationEvents(t), func(tb testing.TB, e workflow.EventDeclareVar) {
					assert.Empty(tb, e.Path.VarScope(),
						assert.MessageF("a Global variable is declared in the root scope, not in %v", declarationVarScope.Get(t)))
				})
			})

			s.Then("the declaration still records the location of the step it was made at", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.OneOf(t, declarationEvents(t), func(tb testing.TB, e workflow.EventDeclareVar) {
					assert.True(tb, e.Path.MatchPrefix(declarationVarScope.Get(t)),
						assert.MessageF("expected %v to be within %v", e.Path, declarationVarScope.Get(t)))
					assert.Must(tb).NotEmpty(e.Path)
					assert.Equal(tb, e.Path[len(e.Path)-1].Name, "declare-var")
				})
			})

			s.Then("the variable is visible from the variable scope the step ran under", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok := lookup(t, declarationVarScope.Get(t))
				assert.True(t, ok)
			})

			s.Then("the variable is visible from an unrelated variable scope", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok := lookup(t, varScopeOf(t.Random.UUID()))
				assert.True(t, ok, "a Global variable must be visible from any variable scope")
			})

			ThenExecutionIsIdempotent(s)
		})
	})
}

func TestSetVar(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		name  = let.As[workflow.VarName](let.String(s))
		value = wftest.LetValue(s)
	)
	subject := let.Var(s, func(t *testcase.T) workflow.SetVar {
		return workflow.SetVar{
			Name:  name.Get(t),
			Value: value.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			Context   = let.Context(s)
			processID = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return c.ActExecuteDefinition(t, Context.Get(t), processID.Get(t), subject.Get(t))
		})

		s.Then("I expect that the process will have the variable set", func(t *testcase.T) {
			act(t)

			assert.Equal[any](t, getVar(t, c.Runtime.Get(t), processID.Get(t), name.Get(t)), value.Get(t))
		})

		s.Then("execution is idempotent with runtime", func(t *testcase.T) {
			assert.NoError(t, act(t)) // first pass

			firstPassEvents := mustHistory(t, c.Runtime.Get(t), processID.Get(t))

			t.Random.Repeat(3, 7, func() {
				assert.NoError(t, act(t))

				assert.Equal(t, mustHistory(t, c.Runtime.Get(t), processID.Get(t)), firstPassEvents)
			})
		})

		s.Then("replaying the step after a later step changed the variable will not restore the old value", func(t *testcase.T) {
			assert.NoError(t, act(t))

			// a later workflow step assigns a new value to the same variable
			var newValue = t.Random.UUID()
			setVar(t, c.Runtime.Get(t), processID.Get(t), name.Get(t), newValue)

			assert.NoError(t, act(t)) // the very same step gets replayed

			assert.Equal[any](t, getVar(t, c.Runtime.Get(t), processID.Get(t), name.Get(t)), newValue,
				"the replayed assignment step must not undo what the steps after it did")
		})
	})
}

func TestDeleteVar(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		name = let.As[workflow.VarName](let.String(s))
	)
	subject := let.Var(s, func(t *testcase.T) workflow.DeleteVar {
		return workflow.DeleteVar{Name: name.Get(t)}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			deletionContext  = let.Context(s)
			deletionVarScope = let.VarOf[workflow.Path](s, nil)
			deletionPath     = let.VarOf[workflow.Path](s, nil)
			processID        = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			ctx := deletionContext.Get(t)
			ctx = withPath(ctx, deletionPath.Get(t))
			ctx = withVarScope(ctx, deletionVarScope.Get(t))
			return c.ActExecuteDefinition(t, ctx, processID.Get(t), subject.Get(t))
		})

		vars := LetVars(s, c.Runtime, processID)

		// declareVarEvent records a variable assignment straight into the event
		// history, standing in for a variable that an earlier workflow step
		// declared and assigned at a given execution path within a given variable
		// scope. It writes the declaration followed by the assignment (which
		// carries the value), mirroring how Vars.set records a first assignment.
		// Both are made at the execution path nested in the variable scope,
		// so the VarScope of their Path is the variable scope.
		//
		// The timestamps are set explicitly because the event history is folded in
		// timestamp order, and the assertions depend on that order. The assignment
		// is stamped strictly after the declaration, because the fold only applies
		// a set to a variable whose declaration is already visible.
		var declareVarEvent = func(t *testcase.T, at time.Time, scope workflow.Path, path workflow.Path, value any) {
			t.Helper()
			var location = slices.Concat(scope, path)
			var declareEvent workflow.Event = workflow.EventDeclareVar{
				EventID:   wftest.MakeEventID(t),
				ProcessID: processID.Get(t),
				Timestamp: at,
				Path:      location,
				Name:      name.Get(t),
			}
			assert.NoError(t, c.EventRepository.Get(t).Create(t.Context(), &declareEvent))

			var setEvent workflow.Event = workflow.EventSetVar{
				EventID:   wftest.MakeEventID(t),
				ProcessID: processID.Get(t),
				Timestamp: at.Add(time.Nanosecond),
				Path:      location,
				Name:      name.Get(t),
				Value:     value,
			}
			assert.NoError(t, c.EventRepository.Get(t).Create(t.Context(), &setEvent))
		}

		// lookup reads the variable under test as it is seen from a given variable scope.
		var lookup = func(t *testcase.T, scope workflow.Path) (any, bool) {
			t.Helper()
			got, ok, err := vars.Get(t).Lookup(withVarScope(t.Context(), scope), name.Get(t))
			assert.NoError(t, err)
			return got, ok
		}

		var deletionEvents = func(t *testcase.T) []workflow.EventDeleteVar {
			t.Helper()
			var out []workflow.EventDeleteVar
			for _, e := range mustHistory(t, c.Runtime.Get(t), processID.Get(t)) {
				if de, ok := e.(workflow.EventDeleteVar); ok {
					out = append(out, de)
				}
			}
			return out
		}

		var ThenExecutionIsIdempotent = func(s *testcase.Spec) {
			s.H().Helper()
			s.Then("execution is idempotent", func(t *testcase.T) {
				assert.NoError(t, act(t)) // first pass

				firstPassEvents := mustHistory(t, c.Runtime.Get(t), processID.Get(t))

				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))

					assert.Equal(t, mustHistory(t, c.Runtime.Get(t), processID.Get(t)), firstPassEvents)
				})
			})
		}

		s.Then("deleting a variable which was never assigned is not an error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.Then("no EventDeleteVar is recorded, since there was nothing to delete", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Empty(t, deletionEvents(t))
		})

		ThenExecutionIsIdempotent(s)

		s.When("the variable is set already", func(s *testcase.Spec) {
			var (
				TopVarScope = let.VarOf[workflow.Path](s, nil)
				TopVarPath  = let.VarOf[workflow.Path](s, nil)
				value       = wftest.LetValue(s)
			)
			s.Before(func(t *testcase.T) {
				declareVarEvent(t, clock.Now().Add(-time.Hour),
					TopVarScope.Get(t), TopVarPath.Get(t), value.Get(t))
			})

			s.Then("it will delete the value", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok := lookup(t, TopVarScope.Get(t))
				assert.False(t, ok)
			})

			s.Then("the EventDeleteVar remembers where the step ran", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.OneOf(t, deletionEvents(t), func(tb testing.TB, e workflow.EventDeleteVar) {
					assert.Equal(tb, e.Name, name.Get(t))
					assert.NotEmpty(tb, e.Path, "the execution path of the deletion step is expected")
					assert.True(tb, e.Path.MatchPrefix(deletionPath.Get(t)))
				})
			})

			ThenExecutionIsIdempotent(s)

			s.Then("replaying the step after a later step reassigned the variable will not delete it again", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok := lookup(t, TopVarScope.Get(t))
				assert.False(t, ok)

				// a later workflow step assigns the variable again
				var newValue = t.Random.UUID()
				setVar(t, c.Runtime.Get(t), processID.Get(t), name.Get(t), newValue)

				assert.NoError(t, act(t)) // the very same step gets replayed

				got, ok := lookup(t, TopVarScope.Get(t))
				assert.True(t, ok, "the replayed deletion step must not remove a value that it never saw")
				assert.Equal[any](t, got, newValue)
			})

			s.And("the variable-scope of the value is outside of the deletion's variable scope", func(s *testcase.Spec) {
				TopVarScope.Let(s, func(t *testcase.T) workflow.Path {
					return varScopeOf("foo", "bar", "baz")
				})

				deletionVarScope.Let(s, func(t *testcase.T) workflow.Path {
					return varScopeOf("baz", "bar", "foo")
				})

				s.Then("deletion won't occur", func(t *testcase.T) {
					assert.NoError(t, act(t))

					got, ok := lookup(t, TopVarScope.Get(t))
					assert.True(t, ok, "the variable must stay intact within its own variable scope")
					assert.Equal[any](t, got, value.Get(t))

					_, ok = lookup(t, deletionVarScope.Get(t))
					assert.False(t, ok, "the variable was never visible from the deletion's variable scope")
				})

				s.Then("no EventDeleteVar is recorded", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Empty(t, deletionEvents(t))
				})

				ThenExecutionIsIdempotent(s)
			})

			s.And("the deletion occurs in a sub variable scope", func(s *testcase.Spec) {
				TopVarScope.Let(s, func(t *testcase.T) workflow.Path {
					return varScopeOf("foo", "bar")
				})

				deletionVarScope.Let(s, func(t *testcase.T) workflow.Path {
					return slicekit.Merge(TopVarScope.Get(t), varScopeOf("qux"))
				})

				s.Then("the sub variable scope deletion which had visibility to the var defined in the top variable scope will delete the variable", func(t *testcase.T) {
					assert.NoError(t, act(t))

					_, ok := lookup(t, deletionVarScope.Get(t))
					assert.False(t, ok)

					_, ok = lookup(t, TopVarScope.Get(t))
					assert.False(t, ok)
				})

				ThenExecutionIsIdempotent(s)

				s.And("variable is REDECLARED in sub variable scope", func(s *testcase.Spec) {
					var SubVarValue = wftest.LetValue(s)

					s.Before(func(t *testcase.T) {
						declareVarEvent(t, clock.Now().Add(-time.Minute),
							deletionVarScope.Get(t),
							slicekit.Merge(TopVarPath.Get(t), pathOf("sub")),
							SubVarValue.Get(t))
					})

					s.Then("deletion will only remove the variable from the current sub var scope", func(t *testcase.T) {
						got, ok := lookup(t, deletionVarScope.Get(t))
						assert.True(t, ok)
						assert.Equal[any](t, got, SubVarValue.Get(t))

						assert.NoError(t, act(t))

						_, ok = lookup(t, deletionVarScope.Get(t))
						assert.False(t, ok)

						got, ok = lookup(t, TopVarScope.Get(t))
						assert.True(t, ok, "the binding of the outer variable scope must survive")
						assert.Equal[any](t, got, value.Get(t))
					})

					ThenExecutionIsIdempotent(s)
				})
			})
		})
	})
}

// withVarScope nests ctx in variable scopes named after the segments of scope, one in another.
func withVarScope(ctx context.Context, scope workflow.Path) context.Context {
	for _, s := range scope {
		ctx = workflow.WithVarScope(ctx, s.Name)
	}
	return ctx
}
