package workflow_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/pkg/txkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestExecute_replayAfterInputWrite(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)
	calls := let.VarOf(s, 0)
	participant, id := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context, int) error {
		return func(ctx context.Context, n int) error {
			calls.Set(t, calls.Get(t)+1)
			vars, err := workflow.GetVars(ctx)
			if err != nil {
				return err
			}
			return vars.Set(ctx, "n", n+1)
		}
	})
	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.SetVar{Name: "n", Value: 0},
			workflow.Execute{Participant: id.Get(t), Input: []workflow.VarName{"n"}},
			workflow.Sleep{},
		}
	})
	s.Test("writing back to a mapped input does not invalidate the completed call", func(t *testcase.T) {
		for range 2 {
			assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
			assert.Equal(t, calls.Get(t), 1)
			assert.Equal(t, len(participantEventsOf(t, c)), 1)
			assert.Equal[any](t, getVar(t, c.Runtime.Get(t), c.ProcessID.Get(t), "n"), 1)
		}
	})
	s.When("the first attempt writes the input and suspends", func(s *testcase.Spec) {
		participant.Let(s, func(t *testcase.T) func(context.Context, int) error {
			write := participant.Super(t)
			return func(ctx context.Context, n int) error {
				if err := write(ctx, n); err != nil {
					return err
				}
				if n == 0 {
					return workflow.Suspend{}
				}
				return nil
			}
		})
		s.Then("replay includes the signaling attempt's write but not the successful call's write", func(t *testcase.T) {
			assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
			assert.Empty(t, participantEventsOf(t, c))
			for range 2 {
				assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), 2)
				events := participantEventsOf(t, c)
				assert.Equal(t, len(events), 1)
				assert.Equal(t, events[0].Input, []any{1})
				assert.Equal[any](t, getVar(t, c.Runtime.Get(t), c.ProcessID.Get(t), "n"), 2)
			}
		})
	})
	s.When("a custom definition remaps the input to another variable", func(s *testcase.Spec) {
		key := let.VarOf[workflow.VarName](s, "n")
		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.SetVar{Name: "n", Value: 0},
				workflow.SetVar{Name: "other", Value: 7},
				wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
					return (workflow.Execute{Participant: id.Get(t), Input: []workflow.VarName{key.Get(t)}}).Execute(ctx, pid)
				}},
				workflow.Sleep{},
			}
		})
		participant.Let(s, func(t *testcase.T) func(context.Context, int) error {
			return func(ctx context.Context, n int) error {
				calls.Set(t, calls.Get(t)+1)
				vars, err := workflow.GetVars(ctx)
				if err != nil {
					return err
				}
				return vars.Set(ctx, "other", n)
			}
		})
		s.Then("a mapping that matched only after the old call misses once", func(t *testcase.T) {
			assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
			assert.Equal(t, calls.Get(t), 1)
			key.Set(t, "other")
			for range 2 {
				assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), 2)
				assert.Equal(t, len(participantEventsOf(t, c)), 2)
			}
		})
	})
}

func TestExecute_replayAfterArgumentMutation(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)
	calls := let.VarOf(s, 0)
	_, id := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context, []string, map[string]int) error {
		return func(_ context.Context, list []string, counts map[string]int) error {
			calls.Set(t, calls.Get(t)+1)
			slices.Sort(list)
			counts["a"]++
			return nil
		}
	})
	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.SetVar{Name: "list", Value: []string{"b", "a"}},
			workflow.SetVar{Name: "counts", Value: map[string]int{"a": 1}},
			workflow.Execute{Participant: id.Get(t), Input: []workflow.VarName{"list", "counts"}},
			workflow.Sleep{},
		}
	})
	s.Test("mutating slice and map arguments changes neither the recorded inputs nor their variables", func(t *testcase.T) {
		for range 2 {
			assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
			assert.Equal(t, calls.Get(t), 1)
			events := participantEventsOf(t, c)
			assert.Equal(t, len(events), 1)
			assert.Equal(t, events[0].Input, []any{[]string{"b", "a"}, map[string]int{"a": 1}})
			assert.Equal[any](t, getVar(t, c.Runtime.Get(t), c.ProcessID.Get(t), "list"), []string{"b", "a"})
			assert.Equal[any](t, getVar(t, c.Runtime.Get(t), c.ProcessID.Get(t), "counts"), map[string]int{"a": 1})
		}
	})
}

func TestExecute_nestedMutableInput(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)
	type input struct {
		Batches [][1][]string
		Labels  [][]string
	}
	value := let.Var(s, func(t *testcase.T) input {
		return input{Batches: [][1][]string{{{"original"}}}}
	})
	c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
		rt := c.Runtime.Super(t)
		codec := wfjson.NewCodec()
		jsonkit.CodecRegisterTypeID[input](codec, "test:nested-input")
		rt.Codec = codec
		return rt
	})
	calls := let.VarOf(s, 0)
	_, id := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context, input) error {
		return func(_ context.Context, in input) error {
			calls.Set(t, calls.Get(t)+1)
			for _, batch := range in.Batches {
				batch[0][0] = "edited"
			}
			for _, labels := range in.Labels {
				labels[0] = "edited"
			}
			return nil
		}
	})
	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.SetVar{Name: "input", Value: value.Get(t)},
			workflow.Execute{Participant: id.Get(t), Input: []workflow.VarName{"input"}},
			workflow.Sleep{},
		}
	})
	checkReplay := func(t *testcase.T) {
		t.Helper()
		rt := c.Runtime.Get(t)
		// Round-trip the variable before the call, both to check that the input
		// is persistable and to get an expectation independent of Go aliases.
		step := workflow.SetVar{Name: "input", Value: value.Get(t)}
		data, err := rt.Codec.Marshal(step)
		assert.NoError(t, err)
		var want workflow.SetVar
		assert.NoError(t, rt.Codec.Unmarshal(data, &want))
		assert.Equal(t, want, step)
		for range 2 {
			assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
			assert.Equal(t, calls.Get(t), 1)
			events := participantEventsOf(t, c)
			assert.Equal(t, len(events), 1)
			assert.Equal(t, events[0].Input, []any{want.Value})
			assert.Equal(t, getVar(t, rt, c.ProcessID.Get(t), "input"), want.Value)
		}
	}
	s.Test("mutating slices inside arrays leaves the call and its variable unchanged on replay", checkReplay)
	s.When("multiple entries refer to the same slice without forming a cycle", func(s *testcase.Spec) {
		value.Let(s, func(t *testcase.T) input {
			return input{Labels: slices.Repeat([][]string{{"original"}}, 16)}
		})
		s.Then("mutating the entries leaves the call and its variable unchanged on replay", checkReplay)
	})
}

func TestExecute_replayAfterBreak(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)
	calls := let.VarOf(s, 0)
	followUp := let.VarOf[workflow.Definition](s, workflow.Break{})
	participant, id := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
		return func(context.Context) error {
			calls.Set(t, calls.Get(t)+1)
			return followUp.Get(t)
		}
	})
	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.For{Do: workflow.Execute{Participant: id.Get(t)}},
			workflow.Sleep{},
		}
	})
	checkReplay := func(t *testcase.T) {
		t.Helper()
		assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
		assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
		assert.Equal(t, calls.Get(t), 1)
		events := participantEventsOf(t, c)
		assert.Equal(t, len(events), 1)
		assert.Equal[workflow.Definition](t, events[0].Definition, workflow.Break{})
	}
	s.Test("both passes leave the loop and suspend without repeating its producer", checkReplay)
	for _, tc := range []struct {
		clause string
		loop   func(workflow.Execute) workflow.For
	}{
		{"Init", func(step workflow.Execute) workflow.For { return workflow.For{Init: step} }},
		{"Cond", func(step workflow.Execute) workflow.For {
			return workflow.For{Cond: wftest.Stub{StubEvaluate: func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
				return false, step.Execute(ctx, pid)
			}}}
		}},
		{"Post", func(step workflow.Execute) workflow.For { return workflow.For{Post: step} }},
	} {
		s.When("the producer runs in For."+tc.clause, func(s *testcase.Spec) {
			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{tc.loop(workflow.Execute{Participant: id.Get(t)}), workflow.Sleep{}}
			})
			s.Then("the clause can break the loop without repeating its producer on replay", checkReplay)
		})
	}
	s.When("a ForEach participant returns a nested participant that breaks", func(s *testcase.Spec) {
		outerCalls := let.VarOf(s, 0)
		_, outer := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
			return func(context.Context) error {
				outerCalls.Set(t, outerCalls.Get(t)+1)
				return workflow.Execute{Participant: id.Get(t)}
			}
		})
		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.SetVar{Name: "items", Value: []string{"one", "two"}},
				workflow.ForEach{Over: "items", Do: workflow.Execute{Participant: outer.Get(t)}},
				workflow.Sleep{},
			}
		})
		s.Then("both calls remain recorded and the second iteration is never run", func(t *testcase.T) {
			for range 2 {
				assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
				assert.Equal(t, calls.Get(t), 1)
				assert.Equal(t, outerCalls.Get(t), 1)
				assert.Equal(t, len(participantEventsOf(t, c)), 2)
			}
		})
	})
	s.When("the follow-up wraps the break in another error", func(s *testcase.Spec) {
		failure := errors.Join(errors.New("follow-up failed"), workflow.Break{})
		followUp.Let(s, func(t *testcase.T) workflow.Definition {
			return wftest.Stub{StubExecute: func(context.Context, workflow.ProcessID) error { return failure }}
		})
		s.Then("the failure escapes the loop and rolls back the producer", func(t *testcase.T) {
			assert.ErrorIs(t, c.ActExecute(t), failure)
			assert.Empty(t, participantEventsOf(t, c))
		})
	})
	s.When("no loop encloses the producer", func(s *testcase.Spec) {
		rollbacks := let.VarOf(s, 0)
		participant.Let(s, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				vars, err := workflow.GetVars(ctx)
				if err != nil {
					return err
				}
				if err := vars.Set(ctx, "partial", true); err != nil {
					return err
				}
				if err := txkit.OnRollback(ctx, func() { rollbacks.Set(t, rollbacks.Get(t)+1) }); err != nil {
					return err
				}
				return workflow.Break{}
			}
		})
		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Execute{Participant: id.Get(t)}
		})
		s.Then("the invalid break rolls back and a corrected participant can finish the same process", func(t *testcase.T) {
			rt, pid := c.Runtime.Get(t), c.ProcessID.Get(t)
			assert.ErrorIs(t, c.ActExecute(t), workflow.Break{})
			assert.False(t, c.IsCompleted(t, pid))
			_, partial, err := getVars(t, rt, pid).Lookup(t.Context(), "partial")
			assert.NoError(t, err)
			assert.Should(t).False(partial)
			assert.Should(t).Empty(participantEventsOf(t, c))
			assert.Should(t).Equal(rollbacks.Get(t), 1)

			correctedCalls := 0
			rt.Participants = workflow.Participants{id.Get(t): func(context.Context) error {
				correctedCalls++
				return nil
			}}
			assert.NoError(t, rt.Execute(t.Context(), pid))
			assert.Equal(t, correctedCalls, 1)
			assert.True(t, c.IsCompleted(t, pid))
			assert.Equal(t, len(participantEventsOf(t, c)), 1)
		})
	})
}

func TestExecute_replayAfterPanic(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)
	cause := errors.New("participant panic")
	rollbacks := let.VarOf(s, 0)
	seenOnEntry := let.VarOf(s, []bool(nil))
	_, leaf := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
		return func(ctx context.Context) error {
			vars, err := workflow.GetVars(ctx)
			if err != nil {
				return err
			}
			_, found, err := vars.Lookup(ctx, "partial")
			if err != nil {
				return err
			}
			testcase.Append(t, seenOnEntry, found)
			if err := vars.Set(ctx, "partial", true); err != nil {
				return err
			}
			if err := txkit.OnRollback(ctx, func() { rollbacks.Set(t, rollbacks.Get(t)+1) }); err != nil {
				return err
			}
			if len(seenOnEntry.Get(t)) == 1 {
				panic(cause)
			}
			return nil
		}
	})
	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Execute{Participant: leaf.Get(t)}
	})
	checkRollback := func(t *testcase.T) {
		t.Helper()
		pid := c.ProcessID.Get(t)
		before := c.ProcessEvents(t, pid)
		recovered := assert.Panic(t, func() { _ = c.ActExecute(t) })
		assert.True(t, recovered == cause, "the original panic value must reach the caller")
		assert.Should(t).Equal(rollbacks.Get(t), 1)
		assert.Should(t).Equal(c.ProcessEvents(t, pid), before)
		assert.NoError(t, c.ActExecute(t))
		assert.Equal(t, seenOnEntry.Get(t), []bool{false, false})
		assert.Equal(t, rollbacks.Get(t), 1)
		assert.True(t, c.IsCompleted(t, pid))
	}
	s.Test("the original panic escapes after rollback, and retry starts clean", checkRollback)
	s.When("the panicking call is a participant's follow-up", func(s *testcase.Spec) {
		outerRollbacks := let.VarOf(s, 0)
		_, outer := wftest.LetParticipant(s, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				if err := txkit.OnRollback(ctx, func() { outerRollbacks.Set(t, outerRollbacks.Get(t)+1) }); err != nil {
					return err
				}
				return workflow.Execute{Participant: leaf.Get(t)}
			}
		})
		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Execute{Participant: outer.Get(t)}
		})
		s.Then("both transactions roll back before the process is retried", func(t *testcase.T) {
			checkRollback(t)
			assert.Equal(t, outerRollbacks.Get(t), 1)
		})
	})
}
