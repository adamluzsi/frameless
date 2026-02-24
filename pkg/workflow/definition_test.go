package workflow_test

import (
	"context"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/visitor"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestPID_smoke(t *testing.T) {
	tc := testcase.NewT(t)
	name := tc.Random.String()
	pid := workflow.PID(name)
	assert.NotNil(t, pid)
	assert.NotEmpty(t, *pid)
	assert.Equal(t, name, string(*pid))
}

func ExampleIf() {
	ifd := &workflow.If{
		Cond: workflow.CID("is-ok"),
		Then: workflow.PID("run-on-true"),
		Else: workflow.PID("run-on-false"),
	}
	_, _, _, _ = ifd, ifd.Cond, ifd.Then, ifd.Else
}

func TestIf(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		c        = letC(s)
		thenStub = c.LetStub(s, "then")
		elseStub = c.LetStub(s, "else")
	)

	var (
		Cond = let.Var(s, func(t *testcase.T) workflow.Condition {
			return workflow.NewConditionTemplate(strconv.FormatBool(t.Random.Bool()))
		})
		Then = let.Var(s, func(t *testcase.T) workflow.Definition {
			return workflow.PID("then")
		})
		Else = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
			return workflow.PID("else")
		})
	)
	subject := let.Var(s, func(t *testcase.T) *workflow.If {
		return &workflow.If{
			Cond: Cond.Get(t),
			Then: Then.Get(t),
			Else: Else.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx   = c.Context.Bind(s)
			state = c.State.Bind(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), state.Get(t))
		})

		s.Test("on a valid If, no error expected from execution", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("fields are missing", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) *workflow.If {
				return &workflow.If{}
			})

			s.Test("on missing fields, we get a validation error", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)

				got, ok := errorkit.As[validate.Error](err)
				assert.True(t, ok)
				assert.Error(t, got.Cause)
			})
		})

		s.When("condition is true", func(s *testcase.Spec) {
			Cond.Let(s, func(t *testcase.T) workflow.Condition {
				return workflow.NewConditionTemplate("true")
			})

			s.And("Then is supplied", func(s *testcase.Spec) {
				Then.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.PID("stub")
				})

				s.Then("If/Then is called", func(t *testcase.T) {
					n := t.Random.Repeat(1, 3, func() {
						assert.NoError(t, act(t))
					})

					assert.Equal(t, thenStub.Get(t).CallCount, n)
					gotCtx, gotState, ok := thenStub.Get(t).LastExecutedWith()
					assert.True(t, ok)
					assert.Equal(t, ctx.Get(t), gotCtx)
					assert.Equal(t, state.Get(t), gotState)
				})
			})

			s.And("Then is absent", func(s *testcase.Spec) {
				Then.Let(s, func(t *testcase.T) workflow.Definition {
					return nil
				})

				s.Then("validation error returned", func(t *testcase.T) {
					err := act(t)
					assert.Error(t, err)

					got, ok := errorkit.As[validate.Error](err)
					assert.True(t, ok)
					assert.NotNil(t, got.Cause)
					assert.Contains(t, got.Cause.Error(), "if.then")
				})
			})
		})

		s.When("condition is false", func(s *testcase.Spec) {
			Cond.Let(s, func(t *testcase.T) workflow.Condition {
				return workflow.NewConditionTemplate("false")
			})

			Then.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.PID("/dev/null")
			})

			s.And("Then is absent", func(s *testcase.Spec) {
				Then.Let(s, func(t *testcase.T) workflow.Definition {
					return nil
				})

				s.Then("validation error returned", func(t *testcase.T) {
					err := act(t)
					assert.Error(t, err)

					got, ok := errorkit.As[validate.Error](err)
					assert.True(t, ok)
					assert.NotNil(t, got.Cause)
					assert.Contains(t, got.Cause.Error(), "if.then")
				})
			})

			s.And("Else is supplied", func(s *testcase.Spec) {
				Else.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.PID("stub")
				})

				s.Then("Else path is executed", func(t *testcase.T) {
					n := t.Random.Repeat(3, 7, func() {
						assert.NoError(t, act(t))
					})

					assert.Equal(t, thenStub.Get(t).CallCount, n)
					gotCtx, gotState, ok := thenStub.Get(t).LastExecutedWith()
					assert.True(t, ok)
					assert.Equal(t, ctx.Get(t), gotCtx)
					assert.Equal(t, state.Get(t), gotState)
				})
			})

			s.And("Else is absent", func(s *testcase.Spec) {
				Else.Let(s, func(t *testcase.T) workflow.Definition {
					return nil
				})

				s.Then("no action is taken", func(t *testcase.T) {
					assert.NoError(t, act(t))
				})
			})
		})
	})

	s.Describe("#Validate", func(s *testcase.Spec) {
		var ctx = c.Context.Bind(s)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Validate(ctx.Get(t))
		})

		s.Then("on a valid if, it yields no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("cond is missing", func(s *testcase.Spec) {
			Cond.LetValue(s, nil)

			s.Then("we get back an error", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "if")
				assert.Contains(t, err.Error(), "cond")
			})
		})

		s.When("then is missing", func(s *testcase.Spec) {
			Then.LetValue(s, nil)

			s.Then("we get back an error", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "if")
				assert.Contains(t, err.Error(), "then")
			})
		})
	})
}

func TestCallParticipant(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		c    = letC(s)
		stub = c.LetStub(s, "stub")
	)

	pid := let.Var(s, func(t *testcase.T) workflow.ParticipantID {
		return workflow.ParticipantID("stub")
	})

	var (
		args = let.VarOf[[]workflow.VariableKey](s, nil)

	)
	subject := let.Var(s, func(t *testcase.T) *workflow.ExecuteParticipant {
		return &workflow.ExecuteParticipant{
			ID:        pid.Get(t),
			Arguments: args.Get(t),
			Return:    returns.Get(t),
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx   = c.Context.Bind(s)
			state = c.State.Bind(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return pid.Get(t).Execute(ctx.Get(t), state.Get(t))
		})

		s.Then("pid is executing the referenced participant", func(t *testcase.T) {
			n := t.Random.Repeat(3, 7, func() {
				assert.NoError(t, act(t))
			})

			assert.Equal(t, stub.Get(t).CallCount, n)
			gotCtx, gotState, ok := stub.Get(t).LastExecutedWith()
			assert.True(t, ok)
			assert.Equal(t, ctx.Get(t), gotCtx)
			assert.Equal(t, state.Get(t), gotState)
		})

		s.When("the pid is invalid in the given context", func(s *testcase.Spec) {
			pid.Let(s, func(t *testcase.T) workflow.ParticipantID {
				validPID := pid.Super(t)
				randomPID := random.Unique(t.Random.String, string(validPID))
				return workflow.ParticipantID(randomPID)
			})

			s.Then("we get back a validation error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrParticipantNotFound{PID: pid.Get(t)})
			})
		})

		s.When("the referenced participant has an issue", func(s *testcase.Spec) {
			expErr := let.Error(s)

			stub.Let(s, func(t *testcase.T) *StubParticipant {
				stub := stub.Super(t)
				stub.Stub = func(ctx context.Context, s *workflow.State) error {
					return expErr.Get(t)
				}
				return stub
			})

			s.Then("error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})
		})
	})

	s.Describe("#Validate", func(s *testcase.Spec) {
		var ctx = c.Context.Bind(s)
		act := let.Act(func(t *testcase.T) error {
			return pid.Get(t).Validate(ctx.Get(t))
		})

		s.Then("on a valid pid, it yields no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("the pid is referencing an unknown participant", func(s *testcase.Spec) {
			pid.Let(s, func(t *testcase.T) workflow.ParticipantID {
				validPID := pid.Super(t)
				randomPID := random.Unique(t.Random.String, string(validPID))
				return workflow.ParticipantID(randomPID)
			})

			s.Then("we get back an error about the unknown participant", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrParticipantNotFound{PID: pid.Get(t)})
			})
		})

		s.When("the pid is an empty string", func(s *testcase.Spec) {
			pid.LetValue(s, "")

			s.Then("we get back an error that we have a empty pid", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "empty")
			})
		})

		s.When("context accidentally don't contain the referenced participant", func(s *testcase.Spec) {
			ctx.Let(s, let.Context(s).Get)

			s.Then("we get back an error about the unknown participant", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.ErrParticipantNotFound{PID: pid.Get(t)})
			})
		})
	})
}

func ExampleSequence() {
	sequence := &workflow.Sequence{
		workflow.PID("foo"),
		workflow.PID("bar"),
		workflow.PID("baz"),
	}

	// a sequence of participants which are executeed after each other.
	_ = sequence
}

func TestSequence(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = letC(s)

	seq := let.Var(s, func(t *testcase.T) workflow.Sequence {
		return workflow.Sequence{}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx   = c.Context.Bind(s)
			state = c.State.Bind(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return seq.Get(t).Execute(ctx.Get(t), state.Get(t))
		})

		s.Test("a valid sequence should yield no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("sequence is empty", func(s *testcase.Spec) {
			seq.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{}
			})

			s.Then("it should yield no error, since nothing can break in it", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("it has an element", func(s *testcase.Spec) {
			foo := c.LetStub(s, "foo")

			seq.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{workflow.PID("foo")}
			})

			s.Then("it should execute the given element", func(t *testcase.T) {
				n := t.Random.Repeat(1, 3, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, foo.Get(t).CallCount, n)
				gotCtx, gotState, ok := foo.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t), gotCtx)
				assert.Equal(t, state.Get(t), gotState)
			})

			s.And("the element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				foo.Let(s, func(t *testcase.T) *StubParticipant {
					v := foo.Super(t)
					v.Err = expErr.Get(t)
					return v
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})
			})
		})

		s.When("it has multiple elements", func(s *testcase.Spec) {
			foo := c.LetStub(s, "foo")
			bar := c.LetStub(s, "bar")
			baz := c.LetStub(s, "baz")

			seq.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{
					workflow.PID("foo"),
					workflow.PID("bar"),
					workflow.PID("baz"),
				}
			})

			s.Then("it should execute all the elements", func(t *testcase.T) {
				n := t.Random.Repeat(1, 3, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, foo.Get(t).CallCount, n)
				gotCtx, gotState, ok := foo.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t), gotCtx)
				assert.Equal(t, state.Get(t), gotState)

				assert.Equal(t, bar.Get(t).CallCount, n)
				gotCtx, gotState, ok = bar.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t), gotCtx)
				assert.Equal(t, state.Get(t), gotState)

				assert.Equal(t, baz.Get(t).CallCount, n)
				gotCtx, gotState, ok = baz.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t), gotCtx)
				assert.Equal(t, state.Get(t), gotState)
			})

			s.And("an element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				bar.Let(s, func(t *testcase.T) *StubParticipant {
					v := foo.Super(t)
					v.Err = expErr.Get(t)
					return v
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})

				s.Then("sequence execution is interrupted by the error", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
					// baz as being the last in the 3 length sequence, is not reached
					assert.Equal(t, baz.Get(t).CallCount, 0)
				})
			})
		})
	})

	s.Describe("#Validate", func(s *testcase.Spec) {
		var ctx = c.Context.Bind(s)
		act := let.Act(func(t *testcase.T) error {
			return seq.Get(t).Validate(ctx.Get(t))
		})

		s.Then("on a valid sequence, it yields no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("one of the elemet has a validation issue", func(s *testcase.Spec) {
			c.LetStub(s, "valid")

			seq.Let(s, func(t *testcase.T) workflow.Sequence {
				return workflow.Sequence{
					workflow.PID("valid"),
					workflow.PID("in-valid"),
					workflow.PID("valid"),
				}
			})

			s.Then("we get back an error", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})
	})
}

func TestVisitor_smoke(t *testing.T) {
	seq := &workflow.Sequence{
		&workflow.If{
			Cond: workflow.CID("if1-cond"),
			Then: workflow.PID("if1-then"),
			Else: workflow.PID("if1-else"),
		},
		&workflow.If{
			Cond: workflow.CID("if2-cond"),
			Then: workflow.PID("if2-then"),
			Else: workflow.PID("if2-else"),
		},
	}

	i := visitor.Walk(workflow.DefinitionPath{}, seq)
	kvs := iterkit.Collect2KV(i)
	assert.Equal(t, 7, len(kvs))

	type KV = iterkit.KV[workflow.DefinitionPath, workflow.Definition]

	assert.OneOf(t, kvs, func(t testing.TB, kv KV) {
		assert.Equal(t, kv.K.ToSlice(), []string{"sequence"})
		assert.Equal[workflow.Definition](t, kv.V, seq)
	})

	assert.OneOf(t, kvs, func(t testing.TB, kv KV) {
		assert.Equal(t, kv.K.ToSlice(), []string{"sequence", "[0]", "if"})
		assert.Equal(t, kv.V, (*seq)[0])
	})

	assert.OneOf(t, kvs, func(t testing.TB, kv KV) {
		assert.Equal(t, kv.K.ToSlice(), []string{"sequence", "[1]", "if"})
		assert.Equal(t, kv.V, (*seq)[1])
	})

	assert.OneOf(t, kvs, func(t testing.TB, kv KV) {
		assert.Equal(t, kv.K.ToSlice(), []string{"sequence", "[0]", "if", "then", "if1-then"})
		assert.Equal(t, kv.V, (*seq)[0].(*workflow.If).Then)
	})

	assert.OneOf(t, kvs, func(t testing.TB, kv KV) {
		assert.Equal(t, kv.K.ToSlice(), []string{"sequence", "[0]", "if", "else", "if1-else"})
		assert.Equal(t, kv.V, (*seq)[0].(*workflow.If).Else)
	})

	assert.OneOf(t, kvs, func(t testing.TB, kv KV) {
		assert.Equal(t, kv.K.ToSlice(), []string{"sequence", "[1]", "if", "then", "if2-then"})
		assert.Equal(t, kv.V, (*seq)[1].(*workflow.If).Then)
	})

	assert.OneOf(t, kvs, func(t testing.TB, kv KV) {
		assert.Equal(t, kv.K.ToSlice(), []string{"sequence", "[1]", "if", "else", "if2-else"})
		assert.Equal(t, kv.V, (*seq)[1].(*workflow.If).Else)
	})
}

func TestConcurrence(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = letC(s)

	con := let.Var(s, func(t *testcase.T) *workflow.Concurrence {
		return &workflow.Concurrence{}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx   = c.Context.Bind(s)
			state = c.State.Bind(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return con.Get(t).Execute(ctx.Get(t), state.Get(t))
		})

		s.Test("a valid sequence should yield no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("sequence is empty", func(s *testcase.Spec) {
			con.Let(s, func(t *testcase.T) *workflow.Concurrence {
				return &workflow.Concurrence{}
			})

			s.Then("it should yield no error, since nothing can break in it", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("it has an element", func(s *testcase.Spec) {
			foo := c.LetStub(s, "foo")

			con.Let(s, func(t *testcase.T) *workflow.Concurrence {
				return &workflow.Concurrence{workflow.PID("foo")}
			})

			s.Then("it should execute the given element", func(t *testcase.T) {
				n := t.Random.Repeat(1, 3, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, foo.Get(t).CallCount, n)
				gotCtx, gotState, ok := foo.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t).Err(), gotCtx.Err())
				assert.Equal(t, state.Get(t), gotState)
			})

			s.And("the element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				foo.Let(s, func(t *testcase.T) *StubParticipant {
					v := foo.Super(t)
					v.Err = expErr.Get(t)
					return v
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})
			})
		})

		s.When("it has multiple elements", func(s *testcase.Spec) {
			foo := c.LetStub(s, "foo")
			bar := c.LetStub(s, "bar")
			baz := c.LetStub(s, "baz")

			con.Let(s, func(t *testcase.T) *workflow.Concurrence {
				return &workflow.Concurrence{
					workflow.PID("foo"),
					workflow.PID("bar"),
					workflow.PID("baz"),
				}
			})

			s.Then("it should execute all the elements", func(t *testcase.T) {
				n := t.Random.Repeat(1, 3, func() {
					assert.NoError(t, act(t))
				})

				assert.Equal(t, foo.Get(t).CallCount, n)
				gotCtx, gotState, ok := foo.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t).Err(), gotCtx.Err())
				assert.Equal(t, state.Get(t), gotState)

				assert.Equal(t, bar.Get(t).CallCount, n)
				gotCtx, gotState, ok = bar.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t).Err(), gotCtx.Err())
				assert.Equal(t, state.Get(t), gotState)

				assert.Equal(t, baz.Get(t).CallCount, n)
				gotCtx, gotState, ok = baz.Get(t).LastExecutedWith()
				assert.True(t, ok)
				assert.Equal(t, ctx.Get(t).Err(), gotCtx.Err())
				assert.Equal(t, state.Get(t), gotState)
			})

			s.When("process definitions manipulate state", func(s *testcase.Spec) {
				foo.Let(s, func(t *testcase.T) *StubParticipant {
					return &StubParticipant{
						Stub: func(ctx context.Context, s *workflow.State) error {
							s.Variables["foo"] = 1
							return nil
						},
					}
				})
				bar.Let(s, func(t *testcase.T) *StubParticipant {
					return &StubParticipant{
						Stub: func(ctx context.Context, s *workflow.State) error {
							s.Variables["bar"] = 2
							return nil
						},
					}
				})
				baz.Let(s, func(t *testcase.T) *StubParticipant {
					return &StubParticipant{
						Stub: func(ctx context.Context, s *workflow.State) error {
							s.Variables["baz"] = 3
							return nil
						},
					}
				})

				s.Then("state is safely merged", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.Equal(t, state.Get(t).Variables["foo"], 1)
					assert.Equal(t, state.Get(t).Variables["bar"], 2)
					assert.Equal(t, state.Get(t).Variables["baz"], 3)
				})
			})

			s.And("an element has an issue", func(s *testcase.Spec) {
				expErr := let.Error(s)

				bar.Let(s, func(t *testcase.T) *StubParticipant {
					v := foo.Super(t)
					v.Err = expErr.Get(t)
					return v
				})

				s.Then("error is propagated back", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
				})

				s.Then("concurrent processes are executed independently", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), expErr.Get(t))
					assert.Equal(t, foo.Get(t).CallCount, 1)
					assert.Equal(t, baz.Get(t).CallCount, 1)

					gotCTX, gotState, ok := foo.Get(t).LastExecutedWith()
					assert.True(t, ok)
					assert.NotNil(t, gotState)
					assert.NoError(t, gotCTX.Err())
				})
			})
		})
	})

	s.Describe("#Validate", func(s *testcase.Spec) {
		var ctx = c.Context.Bind(s)
		act := let.Act(func(t *testcase.T) error {
			return con.Get(t).Validate(ctx.Get(t))
		})

		s.Then("on a valid sequence, it yields no error", func(t *testcase.T) {
			assert.NoError(t, act(t))
		})

		s.When("one of the elemet has a validation issue", func(s *testcase.Spec) {
			c.LetStub(s, "valid")

			con.Let(s, func(t *testcase.T) *workflow.Concurrence {
				return &workflow.Concurrence{
					workflow.PID("valid"),
					workflow.PID("in-valid"),
					workflow.PID("valid"),
				}
			})

			s.Then("we get back an error", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})
	})
}
