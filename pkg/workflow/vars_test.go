package workflow_test

import (
	"context"
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
	var letVarScope = func(s *testcase.Spec) testcase.Var[workflow.VarScope] {
		s.H().Helper()
		return let.Var(s, func(t *testcase.T) workflow.VarScope {
			return random.Slice(t.Random.IntBetween(1, 7), func() string {
				return t.Random.UUID()
			})
		})
	}
	// letPath makes an execution path, one that doesn't open variable scopes
	// along the way.
	var letPath = func(s *testcase.Spec) testcase.Var[workflow.Path] {
		s.H().Helper()
		return let.Var(s, func(t *testcase.T) workflow.Path {
			return random.Slice(t.Random.IntBetween(1, 7), func() string {
				return t.Random.UUID()
			})
		})
	}
	var letVarScopeOther = func(s *testcase.Spec, p testcase.Var[workflow.VarScope]) testcase.Var[workflow.VarScope] {
		return let.Var(s, func(t *testcase.T) workflow.VarScope {
			var o workflow.VarScope = make(workflow.VarScope, len(p.Get(t)))
			for i, v := range p.Get(t) {
				o[i] = random.Unique(t.Random.UUID, v)
			}
			random.Pick(t.Random, func() {
				if 1 < len(o) {
					slicekit.Pop(&o)
				}
			}, func() {
				o = append(o, t.Random.UUID())
			}, func() {
				// do nothing
			})()
			return o
		})
	}

	// declareVar records an explicit declaration of a variable in a given
	// variable scope — the `var name` that brings a binding into existence,
	// independently from any declaration visible from an enclosing scope.
	var declareVar = func(t *testcase.T, name workflow.VarName, scope workflow.VarScope) {
		t.Helper()
		var event workflow.Event = workflow.EventDeclareVar{
			EventID:   mustEventID(t),
			ProcessID: processID.Get(t),
			Timestamp: clock.Now().UTC(),
			Name:      name,
			Scope:     scope,
		}
		assert.NoError(t, eventRepository.Get(t).Create(t.Context(), &event))
	}

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
					assert.Equal(tb, ve.Path, execPath.Get(t))
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
					assert.Equal(tb, ve.Scope, varScope.Get(t))
				})
			})

			s.And("if the variable was already assigned in a previous outer-scope", func(s *testcase.Spec) {
				outerScope := let.Var(s, func(t *testcase.T) workflow.VarScope {
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
						assert.Equal(tb, ve.Scope, varScope.Get(t),
							assert.MessageF("the variable was already declared in the enclosing scope, so no inner re-declaration is expected"))
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
					readScope  = let.Var[workflow.VarScope](s, nil)
					writeScope = let.Var[workflow.VarScope](s, nil)
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
					readScope.Let(s, func(t *testcase.T) workflow.VarScope {
						o := slicekit.Clone(writeScope.Get(t))
						t.Random.Repeat(1, 3, func() {
							o = append(o, t.Random.UUID())
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
					writeScope.Let(s, func(t *testcase.T) workflow.VarScope {
						o := slicekit.Clone(readScope.Get(t))
						t.Random.Repeat(1, 3, func() {
							o = append(o, t.Random.UUID())
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

			s.Context("but the deletion is issued from a variable scope which doesn't have access to the created variable", func(s *testcase.Spec) {
				setContext.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(setContext.PreviousValue(t), workflow.VarScope{"foo", "bar", "baz"})
				})
				ctx.Let(s, func(t *testcase.T) context.Context {
					return withVarScope(setContext.PreviousValue(t), workflow.VarScope{"foo", "bar"})
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
					Scope:     declareScope.Get(t),
				})
				assert.NoError(t, eventRepository.Get(t).Create(t.Context(), &declareEvent))

				setEventID, err := workflow.MakeEventID()
				assert.NoError(t, err)
				setEvent := workflow.Event(workflow.EventSetVar{
					EventID:   setEventID,
					ProcessID: processID.Get(t),
					Timestamp: base.Add(time.Nanosecond),
					Name:      name.Get(t),
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

			outerScope = let.Var(s, func(t *testcase.T) workflow.VarScope {
				return workflow.VarScope{"foo", "bar"}
			})
			innerScope = let.Var(s, func(t *testcase.T) workflow.VarScope {
				return slicekit.Merge(outerScope.Get(t), workflow.VarScope{"baz"})
			})
		)

		// read returns the variable under test as it is seen from a given
		// variable scope.
		//
		// Vars folds the very same event history through two read paths — the
		// single-variable #Lookup and the whole-scope #ToMap — so every scenario
		// below is asserted through both, to keep the two from drifting apart on
		// what a declaration makes visible.
		var read = func(t *testcase.T, scope workflow.VarScope) (any, bool) {
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
			&workflow.ExecuteParticipant{ID: "set_var"},
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
			declarationVarScope = let.VarOf[workflow.VarScope](s, nil)
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
		var lookup = func(t *testcase.T, scope workflow.VarScope) (any, bool) {
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
				assert.Equal(tb, e.Scope, declarationVarScope.Get(t))
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
			declarationVarScope.Let(s, func(t *testcase.T) workflow.VarScope {
				return workflow.VarScope{t.Random.UUID(), t.Random.UUID()}
			})

			s.Then("the declaration is recorded in that variable scope", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.OneOf(t, declarationEvents(t), func(tb testing.TB, e workflow.EventDeclareVar) {
					assert.Equal(tb, e.Scope, declarationVarScope.Get(t))
				})
			})

			s.Then("the variable is not visible from an unrelated variable scope", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok := lookup(t, workflow.VarScope{t.Random.UUID()})
				assert.False(t, ok, "a scoped declaration must not leak out of its own variable scope")
			})

			ThenExecutionIsIdempotent(s)
		})

		s.When("the variable is already declared and assigned in an enclosing variable scope", func(s *testcase.Spec) {
			var (
				enclosingVarScope = let.Var(s, func(t *testcase.T) workflow.VarScope {
					return workflow.VarScope{t.Random.UUID()}
				})
				enclosingValue = wftest.LetValue(s)
			)

			declarationVarScope.Let(s, func(t *testcase.T) workflow.VarScope {
				return slicekit.Merge(enclosingVarScope.Get(t), workflow.VarScope{t.Random.UUID()})
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
			declarationVarScope.Let(s, func(t *testcase.T) workflow.VarScope {
				return workflow.VarScope{t.Random.UUID(), t.Random.UUID()}
			})

			s.Then("the variable is declared in the root scope, not the one the step ran under", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.OneOf(t, declarationEvents(t), func(tb testing.TB, e workflow.EventDeclareVar) {
					assert.Empty(tb, e.Scope,
						assert.MessageF("a Global variable is declared in the root scope, not in %v", declarationVarScope.Get(t)))
				})
			})

			s.Then("the variable is visible from the variable scope the step ran under", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok := lookup(t, declarationVarScope.Get(t))
				assert.True(t, ok)
			})

			s.Then("the variable is visible from an unrelated variable scope", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok := lookup(t, workflow.VarScope{t.Random.UUID()})
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
			deletionVarScope = let.VarOf[workflow.VarScope](s, nil)
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
		// declared and assigned at a given execution path under a given variable
		// scope. It writes the declaration (which carries the scope) followed by
		// the assignment (which carries the value), mirroring how Vars.set records
		// a first assignment.
		//
		// The timestamps are set explicitly because the event history is folded in
		// timestamp order, and the assertions depend on that order. The assignment
		// is stamped strictly after the declaration, because the fold only applies
		// a set to a variable whose declaration is already visible.
		var declareVarEvent = func(t *testcase.T, at time.Time, scope workflow.VarScope, path workflow.Path, value any) {
			t.Helper()
			var declareEvent workflow.Event = workflow.EventDeclareVar{
				EventID:   wftest.MakeEventID(t),
				ProcessID: processID.Get(t),
				Timestamp: at,
				Path:      path,
				Name:      name.Get(t),
				Scope:     scope,
			}
			assert.NoError(t, c.EventRepository.Get(t).Create(t.Context(), &declareEvent))

			var setEvent workflow.Event = workflow.EventSetVar{
				EventID:   wftest.MakeEventID(t),
				ProcessID: processID.Get(t),
				Timestamp: at.Add(time.Nanosecond),
				Path:      path,
				Name:      name.Get(t),
				Value:     value,
			}
			assert.NoError(t, c.EventRepository.Get(t).Create(t.Context(), &setEvent))
		}

		// lookup reads the variable under test as it is seen from a given variable scope.
		var lookup = func(t *testcase.T, scope workflow.VarScope) (any, bool) {
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
				TopVarScope = let.VarOf[workflow.VarScope](s, nil)
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
				TopVarScope.Let(s, func(t *testcase.T) workflow.VarScope {
					return workflow.VarScope{"foo", "bar", "baz"}
				})

				deletionVarScope.Let(s, func(t *testcase.T) workflow.VarScope {
					return workflow.VarScope{"baz", "bar", "foo"}
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
				TopVarScope.Let(s, func(t *testcase.T) workflow.VarScope {
					return workflow.VarScope{"foo", "bar"}
				})

				deletionVarScope.Let(s, func(t *testcase.T) workflow.VarScope {
					return slicekit.Merge(TopVarScope.Get(t), workflow.VarScope{"qux"})
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

func withVarScope(ctx context.Context, scope workflow.VarScope) context.Context {
	for _, name := range scope {
		ctx = workflow.WithVarScope(ctx, name)
	}
	return ctx
}
