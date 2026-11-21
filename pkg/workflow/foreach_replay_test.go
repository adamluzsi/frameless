package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/let"
)

func TestForEach_replay(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)
	seen := let.VarOf[[]any](s, nil)

	s.Test("removing the element does not skip its suspended body", func(t *testcase.T) {
		rt := c.Runtime.Get(t)
		pid, err := execProcess(t, rt, workflow.Sequence{
			workflow.SetVar{Name: "items", Value: []string{"a"}},
			workflow.ForEach{Over: "items", Do: workflow.Sequence{
				workflow.SetVar{Name: "items", Value: []string{}},
				workflow.Sleep{},
			}},
			workflow.SetVar{Name: "after", Value: true},
		})
		assert.ErrorIs(t, err, workflow.Suspend{})
		assert.False(t, c.IsCompleted(t, pid))

		assert.Should(t).ErrorIs(rt.Execute(t.Context(), pid), workflow.Suspend{})
		assert.Should(t).False(c.IsCompleted(t, pid))
		_, ran, err := getVars(t, rt, pid).Lookup(t.Context(), "after")
		assert.NoError(t, err)
		assert.False(t, ran, "the step after the suspended loop must not run")
	})

	s.Test("replaying a finished traversal does not visit appended items", func(t *testcase.T) {
		rt := c.Runtime.Get(t)
		pid, err := execProcess(t, rt, workflow.Sequence{
			workflow.SetVar{Name: "n", Value: 0},
			workflow.SetVar{Name: "items", Value: []string{"a"}},
			workflow.ForEach{Over: "items", Do: workflow.Increment{Name: "n"}},
			workflow.SetVar{Name: "items", Value: []string{"a", "b"}},
			workflow.Sleep{},
		})
		assert.ErrorIs(t, err, workflow.Suspend{})
		assert.Equal[any](t, 1, getVar(t, rt, pid, "n"))
		assert.ErrorIs(t, rt.Execute(t.Context(), pid), workflow.Suspend{})
		assert.Equal[any](t, 1, getVar(t, rt, pid, "n"))
	})

	s.Test("an initially empty traversal stays empty", func(t *testcase.T) {
		rt := c.Runtime.Get(t)
		pid, err := execProcess(t, rt, workflow.Sequence{
			workflow.SetVar{Name: "items", Value: []string{}},
			workflow.ForEach{Over: "items", K: "k", Do: peekVar(t, seen, "k")},
			workflow.SetVar{Name: "items", Value: []string{"late"}},
			workflow.Sleep{},
		})
		assert.ErrorIs(t, err, workflow.Suspend{})
		assert.ErrorIs(t, rt.Execute(t.Context(), pid), workflow.Suspend{})
		assert.Empty(t, seen.Get(t))
	})

	s.Test("the positions replay but K and V reflect replacements and removals", func(t *testcase.T) {
		rt := c.Runtime.Get(t)
		pid, err := execProcess(t, rt, workflow.Sequence{
			workflow.SetVar{Name: "items", Value: map[string]string{"b": "two", "a": "one"}},
			workflow.ForEach{Over: "items", K: "k", V: "v", Do: workflow.Sequence{
				peekVar(t, seen, "k"), peekVar(t, seen, "v"),
			}},
			workflow.Sleep{},
		})
		assert.ErrorIs(t, err, workflow.Suspend{})
		assert.Equal(t, []any{"a", "one", "b", "two"}, seen.Get(t))

		setVar(t, rt, pid, "items", map[string]string{"b": "changed", "c": "new"})
		seen.Set(t, nil)
		assert.ErrorIs(t, rt.Execute(t.Context(), pid), workflow.Suspend{})
		assert.Equal(t, []any{undeclared, undeclared, "b", "changed"}, seen.Get(t))
	})

	s.Test("a For post ranges over the collection set by its sibling body", func(t *testcase.T) {
		var rounds int
		_, err := execProcess(t, c.Runtime.Get(t), workflow.For{
			Cond: wftest.Stub{StubEvaluate: func(context.Context, workflow.ProcessID) (bool, error) {
				rounds++
				return rounds == 1, nil
			}},
			Init: workflow.SetVar{Name: "items", Value: map[string]string{"body": "old"}},
			Do:   workflow.SetVar{Name: "items", Value: map[string]string{"key": "one"}},
			Post: workflow.ForEach{Over: "items", V: "v", Do: peekVar(t, seen, "v")},
		})
		assert.NoError(t, err)
		assert.Equal(t, []any{"one"}, seen.Get(t))
	})

	s.Test("a map break revisits the same key without another increment", func(t *testcase.T) {
		rt := c.Runtime.Get(t)
		pid, err := execProcess(t, rt, workflow.Sequence{
			workflow.SetVar{Name: "n", Value: 0},
			workflow.SetVar{Name: "items", Value: map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7, "h": 8}},
			workflow.ForEach{Over: "items", K: "k", Do: workflow.Sequence{
				peekVar(t, seen, "k"), workflow.Increment{Name: "n"}, workflow.Break{},
			}},
			workflow.Sleep{},
		})
		assert.ErrorIs(t, err, workflow.Suspend{})
		assert.Equal(t, 1, len(seen.Get(t)))
		first := seen.Get(t)[0]
		for range 16 {
			seen.Set(t, nil)
			assert.ErrorIs(t, rt.Execute(t.Context(), pid), workflow.Suspend{})
			assert.Should(t).Equal([]any{first}, seen.Get(t))
			assert.Equal[any](t, 1, getVar(t, rt, pid, "n"))
		}
	})
}

func TestForEach_legacyReplay(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	// Either of these orders was legal over exactly the same map. Seed the
	// old coordinates directly rather than trying to coax MapRange into one.
	for _, tc := range []struct {
		name     string
		keys     []string
		withRefs bool
	}{
		{"iteration references", []string{"z", "m"}, true},
		{"body writes without iteration variables", []string{"m", "z"}, false},
	} {
		s.Test("an old map break keeps the visited order from "+tc.name, func(t *testcase.T) {
			rt, pid := c.Runtime.Get(t), c.ProcessID.Get(t)
			ctx := workflow.WithName(rt.Context(t.Context()), "loop")
			setVar(t, rt, pid, "items", map[string]int{"a": 1, "m": 2, "z": 3})
			setVar(t, rt, pid, "n", 0)
			loop := workflow.ForEach{Over: "items"}
			var body workflow.Sequence
			if tc.withRefs {
				loop.K = "k"
			} else {
				body = append(body, workflow.Increment{Name: "n"})
			}
			for _, key := range tc.keys {
				at := workflow.WithVarScope(ctx, key)
				if tc.withRefs {
					var e workflow.Event = workflow.EventForEachIteration{
						EventID: mustEventID(t), ProcessID: pid, Timestamp: clock.Now().UTC(),
						Path: workflow.CurrentPath(at), Over: loop.Over, K: loop.K,
					}
					assert.NoError(t, rt.Events.Create(ctx, &e))
				}
				assert.NoError(t, body.Execute(at, pid))
			}
			count := getVar(t, rt, pid, "n")
			var walked []string
			loop.Do = append(body, wftest.Stub{StubExecute: func(ctx context.Context, _ workflow.ProcessID) error {
				scope := workflow.CurrentPath(ctx).VarScope()
				key := scope[len(scope)-1].Name
				walked = append(walked, key)
				if key == tc.keys[1] {
					return workflow.Break{}
				}
				return nil
			}})
			for range 2 {
				walked = nil
				assert.NoError(t, loop.Execute(ctx, pid))
				assert.Equal(t, tc.keys, walked)
				assert.Equal(t, count, getVar(t, rt, pid, "n"))
			}
			assert.Equal(t, 1, len(eventsOfType[workflow.EventForEachTraversal](t, rt.Events, pid)))
		})
	}

	s.Test("a recorded visit survives even when the old collection cannot be reconstructed", func(t *testcase.T) {
		rt, pid := c.Runtime.Get(t), c.ProcessID.Get(t)
		ctx := workflow.WithName(rt.Context(t.Context()), "loop")
		var ref workflow.Event = workflow.EventForEachIteration{
			EventID: mustEventID(t), ProcessID: pid, Timestamp: clock.Now().UTC(),
			Path: workflow.CurrentPath(workflow.WithVarScope(ctx, "z")), Over: "items", K: "k",
		}
		assert.NoError(t, rt.Events.Create(ctx, &ref))
		setVar(t, rt, pid, "items", []string{"new collection"})

		var walked []string
		loop := workflow.ForEach{Over: "items", K: "k", Do: wftest.Stub{StubExecute: func(ctx context.Context, _ workflow.ProcessID) error {
			scope := workflow.CurrentPath(ctx)
			walked = append(walked, scope[len(scope)-1].Name)
			return workflow.Break{}
		}}}
		assert.NoError(t, loop.Execute(ctx, pid))
		assert.Equal(t, []string{"z"}, walked)
	})

	s.Test("an old partial slice walk uses the collection before its first body write", func(t *testcase.T) {
		rt, pid := c.Runtime.Get(t), c.ProcessID.Get(t)
		ctx := workflow.WithName(rt.Context(t.Context()), "loop")
		setVar(t, rt, pid, "items", []string{"a", "b"})
		body := workflow.Sequence{workflow.DeleteVar{Name: "items"}}
		assert.NoError(t, body.Execute(workflow.WithVarScope(ctx, "[0]"), pid))

		var walked []string
		loop := workflow.ForEach{Over: "items", Do: append(body, wftest.Stub{StubExecute: func(ctx context.Context, _ workflow.ProcessID) error {
			scope := workflow.CurrentPath(ctx).VarScope()
			walked = append(walked, scope[len(scope)-1].Name)
			return nil
		}})}
		assert.NoError(t, loop.Execute(ctx, pid))
		assert.Equal(t, []string{"[0]", "[1]"}, walked)
	})
}
