package wfcontract_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfcontract"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

// TwoLoops runs two ForEach without giving them a path segment of their own,
// so the iterations of the second loop land on the paths of the first one's.
type TwoLoops struct{ First, Second workflow.ForEach }

func (TwoLoops) Error() string { return "wfcontract_test::two-loops" }

func (d TwoLoops) Execute(ctx context.Context, pid workflow.ProcessID) error {
	if err := d.First.Execute(ctx, pid); err != nil {
		return err
	}
	return d.Second.Execute(ctx, pid) // same ctx: same Path as First
}

// Forward runs its Definition without giving it a path segment of its own.
type Forward struct{ Definition workflow.Definition }

func (Forward) Error() string { return "wfcontract_test::forward" }

func (d Forward) Execute(ctx context.Context, pid workflow.ProcessID) error {
	return d.Definition.Execute(ctx, pid) // the path of Forward itself
}

// Elsewhere runs its Definition in a process of its own, instead of the process it is executed in.
type Elsewhere struct{ Definition workflow.Definition }

func (Elsewhere) Error() string { return "wfcontract_test::elsewhere" }

func (d Elsewhere) Execute(ctx context.Context, _ workflow.ProcessID) error {
	other, err := workflow.MakeProcessID()
	if err != nil {
		return err
	}
	return d.Definition.Execute(workflow.WithName(ctx, "elsewhere"), other)
}

// Noop is a custom definition that does nothing.
type Noop struct{}

func (Noop) Error() string { return "wfcontract_test::noop" }

func (Noop) Execute(context.Context, workflow.ProcessID) error { return nil }

// Point is a struct that a JSON codec turns into a map when it is held by an any value.
type Point struct{ X, Y int }

// SetPoint records a Point as the value of the "point" variable.
type SetPoint struct{}

func (SetPoint) Error() string { return "wfcontract_test::set-point" }

func (SetPoint) Execute(ctx context.Context, pid workflow.ProcessID) error {
	return workflow.SetVar{Name: "point", Value: Point{X: 1, Y: 2}}.
		Execute(workflow.WithName(ctx, "set-point"), pid)
}

func TestDefinition(t *testing.T) {
	s := testcase.NewSpec(t)

	// mk makes the Definition that the contract is run against.
	mk := let.Var(s, func(t *testcase.T) func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
		return func(tb testing.TB, c wfcontract.DefinitionContext) workflow.Definition {
			return workflow.Sequence{c.MakeDefinition(tb), c.MakeDefinition(tb)}
		}
	})

	// codec knows the definitions of this test, except Noop.
	codec := let.Var(s, func(t *testcase.T) *jsonkit.Codec {
		c := wfjson.NewCodec()
		jsonkit.CodecRegisterTypeID[TwoLoops](c, "wfcontract_test::two-loops")
		jsonkit.CodecRegisterTypeID[Forward](c, "wfcontract_test::forward")
		jsonkit.CodecRegisterTypeID[Elsewhere](c, "wfcontract_test::elsewhere")
		jsonkit.CodecRegisterTypeID[SetPoint](c, "wfcontract_test::set-point")
		return c
	})

	config := let.Var(s, func(t *testcase.T) wfcontract.DefinitionConfig {
		return wfcontract.DefinitionConfig{Codec: codec.Get(t)}
	})

	// act runs the contract against the Definition, and reports how it went.
	act := func(t *testcase.T) *testcase.FakeTB {
		dtb := &testcase.FakeTB{}
		spec := testcase.NewSpec(dtb)
		wfcontract.Definition(mk.Get(t), config.Get(t)).Spec(spec)
		testcase.Sandbox(spec.Finish)
		return dtb
	}

	s.Then("a definition that gives each sub definition a path of its own passes", func(t *testcase.T) {
		dtb := act(t)
		assert.False(t, dtb.IsFailed, assert.Message(dtb.Logs.String()))
	})

	s.When("no codec is configured", func(s *testcase.Spec) {
		config.Let(s, func(t *testcase.T) wfcontract.DefinitionConfig {
			return wfcontract.DefinitionConfig{}
		})

		s.Then("the contract passes with the default codec", func(t *testcase.T) {
			dtb := act(t)
			assert.False(t, dtb.IsFailed, assert.Message(dtb.Logs.String()))
		})
	})

	s.When("the definition is not registered on the codec", func(s *testcase.Spec) {
		mk.Let(s, func(t *testcase.T) func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
			return func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
				return Noop{}
			}
		})

		s.Then("the contract fails on its round-trip through the codec", func(t *testcase.T) {
			dtb := act(t)
			assert.True(t, dtb.IsFailed)
			assert.Contains(t, dtb.Logs.String(), "wfcontract_test.Noop to round-trip through the codec")
		})

		s.And("it is registered on the configured codec", func(s *testcase.Spec) {
			codec.Let(s, func(t *testcase.T) *jsonkit.Codec {
				c := codec.Super(t)
				jsonkit.CodecRegisterTypeID[Noop](c, "wfcontract_test::noop")
				return c
			})

			s.Then("the contract passes", func(t *testcase.T) {
				dtb := act(t)
				assert.False(t, dtb.IsFailed, assert.Message(dtb.Logs.String()))
			})
		})
	})

	s.When("the definition records an event that doesn't round-trip through the codec", func(s *testcase.Spec) {
		mk.Let(s, func(t *testcase.T) func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
			return func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
				return SetPoint{}
			}
		})

		s.Then("the contract fails on the round-trip of the event", func(t *testcase.T) {
			dtb := act(t)
			assert.True(t, dtb.IsFailed)
			assert.Contains(t, dtb.Logs.String(), "workflow.EventSetVar to round-trip through the codec")
		})
	})

	s.When("the definition executes its sub definition in another process", func(s *testcase.Spec) {
		mk.Let(s, func(t *testcase.T) func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
			return func(tb testing.TB, c wfcontract.DefinitionContext) workflow.Definition {
				return Elsewhere{Definition: c.MakeDefinition(tb)}
			}
		})

		s.Then("the contract fails on the process of the sub definition", func(t *testcase.T) {
			dtb := act(t)
			assert.True(t, dtb.IsFailed)
			assert.Contains(t, dtb.Logs.String(), "outside of the process of the definition")
		})
	})

	s.When("the definition executes two sub definitions at the same path", func(s *testcase.Spec) {
		mk.Let(s, func(t *testcase.T) func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
			return func(tb testing.TB, c wfcontract.DefinitionContext) workflow.Definition {
				return workflow.Sequence{
					workflow.SetVar{Name: "letters", Value: []any{"a", "b"}},
					workflow.SetVar{Name: "numbers", Value: []any{1.0, 2.0}},
					TwoLoops{
						First:  workflow.ForEach{Over: "letters", V: "letter", Do: c.MakeDefinition(tb)},
						Second: workflow.ForEach{Over: "numbers", V: "number", Do: c.MakeDefinition(tb)},
					},
				}
			}
		})

		s.Then("the contract fails on the shared path", func(t *testcase.T) {
			dtb := act(t)
			assert.True(t, dtb.IsFailed)
			assert.Contains(t, dtb.Logs.String(), "share the path")
		})
	})

	s.When("the definition executes its sub definition at its own path", func(s *testcase.Spec) {
		mk.Let(s, func(t *testcase.T) func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
			return func(tb testing.TB, c wfcontract.DefinitionContext) workflow.Definition {
				return Forward{Definition: c.MakeDefinition(tb)}
			}
		})

		s.Then("the contract fails on the missing path segment", func(t *testcase.T) {
			dtb := act(t)
			assert.True(t, dtb.IsFailed)
			assert.Contains(t, dtb.Logs.String(), "at the path of the definition itself")
		})
	})

	s.When("a sub definition is executed within the path of another", func(s *testcase.Spec) {
		// For runs its Post at for/[n], and its Do within that, at for/[n]/body.
		mk.Let(s, func(t *testcase.T) func(testing.TB, wfcontract.DefinitionContext) workflow.Definition {
			return func(tb testing.TB, c wfcontract.DefinitionContext) workflow.Definition {
				return workflow.For{
					Init: workflow.SetVar{Name: "i", Value: 0},
					Cond: wftemplate.Condition(`lt .i 2`),
					Do:   workflow.Sequence{workflow.Increment{Name: "i"}, c.MakeDefinition(tb)},
					Post: c.MakeDefinition(tb),
				}
			}
		})

		s.Then("the contract passes, as the paths still differ", func(t *testcase.T) {
			dtb := act(t)
			assert.False(t, dtb.IsFailed, assert.Message(dtb.Logs.String()))
		})
	})
}
