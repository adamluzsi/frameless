package workflow_test

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleIncrement() {
	// A declared variable needs no initial value to serve as a counter.
	_ = workflow.Sequence{
		workflow.DeclareVar{Name: "attempts"},
		workflow.Increment{Name: "attempts"}, // attempts == 1
		workflow.Increment{Name: "attempts"}, // attempts == 2
	}
}

func TestIncrement(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	var (
		name = let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.StringNC(5, random.CharsetAlpha()))
		})
	)

	subject := let.Var(s, func(t *testcase.T) workflow.Increment {
		return workflow.Increment{Name: name.Get(t)}
	})

	// countSetVarEvents counts the EventSetVar entries recorded for a given
	// variable in a Process history.
	//
	// The event log is the single source of truth, so "the increment happened
	// once" is a claim about how many mutation events were appended, not only
	// about the value that a final read happens to return.
	var countSetVarEvents = func(events []workflow.Event, name workflow.VarName) int {
		var n int
		for _, e := range events {
			if e, ok := e.(workflow.EventSetVar); ok && e.Name == name {
				n++
			}
		}
		return n
	}

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)

			// value is what the variable holds at the moment of the increment.
			value = let.Var[any](s, func(t *testcase.T) any {
				return t.Random.IntBetween(0, 100)
			})
			// expected is what the variable must hold afterwards. It is kept
			// next to value so that a context which changes the variable's type
			// has to state the incremented value in that same type.
			expected = let.Var[any](s, func(t *testcase.T) any {
				return value.Get(t).(int) + 1
			})
		)

		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		var vars = func(t *testcase.T) workflow.Vars {
			return workflow.Vars{
				ProcessID:        process.Get(t),
				EventsRepository: c.EventRepository.Get(t),
			}
		}

		var lookup = func(t *testcase.T) (any, bool) {
			t.Helper()
			got, ok, err := vars(t).Lookup(ctx.Get(t), name.Get(t))
			assert.NoError(t, err)
			return got, ok
		}

		// Seeding the binding is Process state, not a context variant. What the
		// arrangement does is a total function of `value`: a nil value means the
		// variable is declared but never assigned, which is precisely the state
		// `DeclareVar` on its own leaves behind.
		s.Before(func(t *testcase.T) {
			if v := value.Get(t); v != nil {
				assert.NoError(t, vars(t).Set(ctx.Get(t), name.Get(t), v))
				return
			}
			assert.NoError(t, workflow.DeclareVar{Name: name.Get(t)}.
				Execute(ctx.Get(t), process.Get(t)))
		})

		s.Then("the variable is incremented by one", func(t *testcase.T) {
			assert.NoError(t, act(t))

			got, ok := lookup(t)
			assert.True(t, ok)
			assert.Equal[any](t, got, expected.Get(t))
		})

		s.Then("the incremented value keeps the variable's original type", func(t *testcase.T) {
			assert.NoError(t, act(t))

			got, ok := lookup(t)
			assert.True(t, ok)
			assert.Equal(t,
				reflect.TypeOf(got).String(),
				reflect.TypeOf(value.Get(t)).String(),
				"the increment must yield the value itself, not a reflection wrapper around it")
		})

		s.Then("executing it again will not increment the variable a second time", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))

			got, ok := lookup(t)
			assert.True(t, ok)
			assert.Equal[any](t, got, expected.Get(t),
				"a replayed increment at the same path is the same increment")
		})

		s.Then("a repeated execution appends no further mutation to the event log", func(t *testcase.T) {
			assert.NoError(t, act(t))

			before := countSetVarEvents(c.ProcessEvents(t, process.Get(t)), name.Get(t))

			assert.NoError(t, act(t))

			after := countSetVarEvents(c.ProcessEvents(t, process.Get(t)), name.Get(t))
			assert.Equal(t, before, after,
				"replaying a step must not grow the event log")
		})

		s.When("the variable is declared but has no value assigned", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any { return nil })
			expected.Let(s, func(t *testcase.T) any { return 1 })

			s.Then("the increment counts up from a zero int", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok := lookup(t)
				assert.True(t, ok)
				assert.Equal[any](t, got, expected.Get(t),
					"a declaration alone is enough to start counting")
			})

			s.Then("the assumed zero is an int", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok := lookup(t)
				assert.True(t, ok)
				assert.Equal(t, reflect.TypeOf(got).String(), "int")
			})

			s.Then("executing it again will not increment a second time", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))

				got, ok := lookup(t)
				assert.True(t, ok)
				assert.Equal[any](t, got, expected.Get(t))
			})
		})

		// whenVariableIs declares a context where the variable holds a given
		// numeric type, and expects the increment to stay within that type.
		//
		// Go's ++ operator is defined for every numeric type, so the increment
		// operator is specified against that same set rather than against int
		// alone. float64 in particular is not academic: a value that has been
		// through a JSON-backed event repository comes back as a float64.
		var whenVariableIs = func(desc string, val func(t *testcase.T) any, next func(v any) any) {
			s.When("the variable holds a "+desc, func(s *testcase.Spec) {
				value.Let(s, func(t *testcase.T) any { return val(t) })
				expected.Let(s, func(t *testcase.T) any { return next(value.Get(t)) })

				s.Then("it is incremented as a "+desc, func(t *testcase.T) {
					assert.NoError(t, act(t))

					got, ok := lookup(t)
					assert.True(t, ok)
					assert.Equal[any](t, got, expected.Get(t))
				})
			})
		}

		var smallInt = func(t *testcase.T) int { return t.Random.IntBetween(0, 100) }

		whenVariableIs("int8",
			func(t *testcase.T) any { return int8(smallInt(t)) },
			func(v any) any { return v.(int8) + 1 })
		whenVariableIs("int16",
			func(t *testcase.T) any { return int16(smallInt(t)) },
			func(v any) any { return v.(int16) + 1 })
		whenVariableIs("int32",
			func(t *testcase.T) any { return int32(smallInt(t)) },
			func(v any) any { return v.(int32) + 1 })
		whenVariableIs("int64",
			func(t *testcase.T) any { return int64(smallInt(t)) },
			func(v any) any { return v.(int64) + 1 })
		whenVariableIs("uint",
			func(t *testcase.T) any { return uint(smallInt(t)) },
			func(v any) any { return v.(uint) + 1 })
		whenVariableIs("uint8",
			func(t *testcase.T) any { return uint8(smallInt(t)) },
			func(v any) any { return v.(uint8) + 1 })
		whenVariableIs("uint64",
			func(t *testcase.T) any { return uint64(smallInt(t)) },
			func(v any) any { return v.(uint64) + 1 })
		whenVariableIs("float32",
			func(t *testcase.T) any { return float32(smallInt(t)) },
			func(v any) any { return v.(float32) + 1 })
		whenVariableIs("float64",
			func(t *testcase.T) any { return float64(smallInt(t)) },
			func(v any) any { return v.(float64) + 1 })

		s.When("the variable holds a named integer type", func(s *testcase.Spec) {
			type Counter int

			value.Let(s, func(t *testcase.T) any {
				return Counter(t.Random.IntBetween(0, 100))
			})
			expected.Let(s, func(t *testcase.T) any {
				return value.Get(t).(Counter) + 1
			})

			s.Then("it is incremented without losing the named type", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok := lookup(t)
				assert.True(t, ok)
				assert.Equal[any](t, got, expected.Get(t))
			})
		})

		s.When("the variable holds a value that Go's ++ operator does not accept", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) any {
				return t.Random.String()
			})

			s.Then("a fatal error is returned", func(t *testcase.T) {
				assert.Error(t, act(t))
				assert.True(t, workflow.ErrIsFatal(act(t)),
					"incrementing a non numeric value is a definition mistake, not a transient failure")
			})

			s.Then("the variable is left untouched", func(t *testcase.T) {
				assert.Error(t, act(t))

				got, ok := lookup(t)
				assert.True(t, ok)
				assert.Equal[any](t, got, value.Get(t))
			})
		})

		s.When("the variable is not declared", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.Increment {
				return workflow.Increment{Name: workflow.VarName(t.Random.StringNC(7, random.CharsetAlpha()))}
			})

			s.Then("a fatal error is returned", func(t *testcase.T) {
				assert.Error(t, act(t))
				assert.True(t, workflow.ErrIsFatal(act(t)),
					"incrementing an undeclared variable can never succeed on a retry")
			})
		})
	})

	s.Context("as part of a Definition", func(s *testcase.Spec) {
		s.Test("a declared counter is incremented when the Process runs", func(t *testcase.T) {
			def := workflow.Sequence{
				workflow.DeclareVar{Name: "n"},
				workflow.SetVar{Name: "n", Value: 0},
				workflow.Increment{Name: "n"},
			}

			rt := c.Runtime.Get(t)
			pid := wftest.MakeProcessID(t)
			assert.NoError(t, rt.Bind(t.Context(), pid, def))
			assert.NoError(t, rt.Execute(t.Context(), pid))

			assert.Equal[any](t, getVar(t, rt, pid, "n"), 1)
		})

		s.Test("re-executing a Process does not increment the counter again", func(t *testcase.T) {
			def := workflow.Sequence{
				workflow.DeclareVar{Name: "n"},
				workflow.SetVar{Name: "n", Value: 0},
				workflow.Increment{Name: "n"},
			}

			rt := c.Runtime.Get(t)
			pid := wftest.MakeProcessID(t)
			assert.NoError(t, rt.Bind(t.Context(), pid, def))

			assert.NoError(t, rt.Execute(t.Context(), pid))
			assert.NoError(t, rt.Execute(t.Context(), pid))

			assert.Equal[any](t, getVar(t, rt, pid, "n"), 1,
				"replaying the Process must arrive at the same state, not a further increment")
		})

		s.Test("a declared but unassigned counter starts from zero", func(t *testcase.T) {
			def := workflow.Sequence{
				workflow.DeclareVar{Name: "i"},
				workflow.Increment{Name: "i"},
				workflow.Increment{Name: "i"},
			}

			rt := c.Runtime.Get(t)
			pid := wftest.MakeProcessID(t)
			assert.NoError(t, rt.Bind(t.Context(), pid, def))
			assert.NoError(t, rt.Execute(t.Context(), pid))

			assert.Equal[any](t, getVar(t, rt, pid, "i"), 2,
				"declaring a variable is enough to use it as a counter")
		})

		s.Test("an increment in a loop body runs once per iteration", func(t *testcase.T) {
			elements := []string{"foo", "bar", "baz"}

			def := workflow.Sequence{
				workflow.DeclareVar{Name: "n"},
				workflow.SetVar{Name: "n", Value: 0},
				workflow.DeclareVar{Name: "vs"},
				workflow.SetVar{Name: "vs", Value: elements},
				workflow.ForEach{
					Over: "vs", V: "e",
					Do: workflow.Increment{Name: "n"},
				},
			}

			rt := c.Runtime.Get(t)
			pid := wftest.MakeProcessID(t)
			assert.NoError(t, rt.Bind(t.Context(), pid, def))
			assert.NoError(t, rt.Execute(t.Context(), pid))

			assert.Equal[any](t, getVar(t, rt, pid, "n"), len(elements),
				"each iteration is its own step, so idempotency must not collapse them into one")
		})
	})
}
