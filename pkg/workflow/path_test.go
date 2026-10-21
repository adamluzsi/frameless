package workflow_test

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/frameless/pkg/workflow/wftest"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleWithName() {
	ctx := workflow.WithName(context.Background(), "root")
	ctx = workflow.WithName(ctx, "child")
	_ = workflow.CurrentPath(ctx) // root/child
}

func ExampleWithVarScope() {
	ctx := workflow.WithName(context.Background(), "root")
	ctx = workflow.WithVarScope(ctx, "block")
	ctx = workflow.WithName(ctx, "step")
	path := workflow.CurrentPath(ctx) // root/block/step, where block is a variable scope
	_ = path.VarScope()               // root/block
}

// pathOf builds a workflow.Path out of plain scope names, none of them a variable scope.
func pathOf(names ...string) workflow.Path {
	var p = make(workflow.Path, 0, len(names))
	for _, name := range names {
		p = append(p, workflow.Scope{Name: name})
	}
	return p
}

// varScopeOf builds a workflow.Path in which every scope is a variable scope,
// like the one WithVarScope makes out of the same names.
func varScopeOf(names ...string) workflow.Path {
	var p = make(workflow.Path, 0, len(names))
	for _, name := range names {
		p = append(p, workflow.Scope{Name: name, Var: true})
	}
	return p
}

// assertSamePath asserts that two Paths are the same Scope by Scope,
// whether a Scope is a variable scope included.
//
// workflow.Scope#Equal identifies a Scope by its name alone,
// so assert.Equal on Paths doesn't tell their variable scopes apart.
func assertSamePath(tb testing.TB, exp, got workflow.Path, msg ...assert.Message) {
	tb.Helper()
	assert.True(tb, slices.Equal(exp, got), append(msg, assert.MessageF("expected: %#v\ngot:      %#v", exp, got))...)
}

// letRandomPath returns a random Path of the given length, with unique scope names,
// some of which are variable scopes.
func letRandomPath(t *testcase.T, length int) workflow.Path {
	return random.Slice(length, func() workflow.Scope {
		return workflow.Scope{Name: t.Random.String(), Var: t.Random.Bool()}
	})
}

func TestCurrentPath(t *testing.T) {
	s := testcase.NewSpec(t)

	var Context = let.Var(s, func(t *testcase.T) context.Context {
		return t.Context()
	})
	act := let.Act(func(t *testcase.T) workflow.Path {
		return workflow.CurrentPath(Context.Get(t))
	})

	s.Then("it returns a Path without an issue", func(t *testcase.T) {
		var _ workflow.Path = act(t)
	})

	s.Context("when context", func(s *testcase.Spec) {
		s.Context("is nil", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				return nil
			})

			s.Then("it returns an empty current path", func(t *testcase.T) {
				assert.Empty(t, act(t))
			})
		})

		s.Context("doesn't have previous path entries", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				return context.Background()
			})

			s.Then("it returns an empty path", func(t *testcase.T) {
				assert.Empty(t, act(t))
			})
		})

		s.Context("contains contains path with using workflow.WithName", func(s *testcase.Spec) {
			expectedPath := let.Var(s, func(t *testcase.T) workflow.Path {
				return pathOf(random.Slice(t.Random.IntBetween(1, 7), t.Random.String)...)
			})

			Levels := let.Var(s, func(t *testcase.T) []context.Context {
				var levels = make([]context.Context, len(expectedPath.Get(t)))
				var ctx = context.Background()
				for i, scope := range expectedPath.Get(t) {
					ctx = workflow.WithName(ctx, scope.Name)
					levels[i] = ctx
				}
				return levels
			})

			Context.Let(s, func(t *testcase.T) context.Context {
				_ = slicekit.Last[any]
				last, ok := slicekit.Last(Levels.Get(t))
				assert.True(t, ok)
				return last
			})

			s.Then("it returns the current path", func(t *testcase.T) {
				assertSamePath(t, expectedPath.Get(t), act(t))
			})

			s.Then("each context level keeps its own state intact", func(t *testcase.T) {
				for i := range expectedPath.Get(t) {
					var currentPath = workflow.CurrentPath(Levels.Get(t)[i])
					var expectedPath = expectedPath.Get(t)[:i+1]
					assert.Equal(t, expectedPath, currentPath)
				}
			})

			s.Then("it is in the root variable scope", func(t *testcase.T) {
				assert.Empty(t, act(t).VarScope())
			})
		})
		s.Context("contains a variable scope made with workflow.WithVarScope", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := workflow.WithName(t.Context(), "sequence")
				ctx = workflow.WithVarScope(ctx, "loop")
				return workflow.WithName(ctx, "participant")
			})

			s.Then("the variable scope is a segment of the path, marked as a variable scope", func(t *testcase.T) {
				assertSamePath(t, workflow.Path{
					{Name: "sequence"},
					{Name: "loop", Var: true},
					{Name: "participant"},
				}, act(t))
			})
		})
	})

	s.Context("smoke", func(s *testcase.Spec) {
		s.Test("context based level", func(t *testcase.T) {
			var (
				ctx0  = context.Background()
				ctx1  = workflow.WithName(ctx0, "foo")
				ctx2  = workflow.WithName(ctx1, "bar")
				ctx2B = workflow.WithName(ctx1, "bar-2")
				ctx3  = workflow.WithName(ctx2, "baz")
			)

			assert.Equal(t, workflow.CurrentPath(ctx3), pathOf("foo", "bar", "baz"))
			assert.Equal(t, workflow.CurrentPath(ctx2), pathOf("foo", "bar"))
			assert.Equal(t, workflow.CurrentPath(ctx1), pathOf("foo"))
			assert.Equal(t, workflow.CurrentPath(ctx2B), pathOf("foo", "bar-2"))
			assert.Empty(t, workflow.CurrentPath(ctx0))
		})
	})
}

//-------------------------------------------------------------------------------------------------

// pathRecorder is a thin, concurrency-safe wrapper around a slice of paths
// captured via workflow.Path(ctx). It lets tests assert what each Definition
// observed in the ctx it received during Execute.
type pathRecorder struct {
	m     sync.Mutex
	paths []workflow.Path
}

func (r *pathRecorder) Record(ctx context.Context) {
	r.m.Lock()
	defer r.m.Unlock()
	r.paths = append(r.paths, append(workflow.Path(nil), workflow.CurrentPath(ctx)...))
}

func (r *pathRecorder) Snapshot() []workflow.Path {
	r.m.Lock()
	defer r.m.Unlock()
	out := make([]workflow.Path, len(r.paths))
	for i, p := range r.paths {
		out[i] = append(workflow.Path(nil), p...)
	}
	return out
}

func TestExecute_participantPathSegment(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	rec := let.Var(s, func(t *testcase.T) *pathRecorder {
		return &pathRecorder{}
	})

	_, pid := wftest.LetParticipant[func(context.Context) error](s, func(t *testcase.T) func(context.Context) error {
		return func(ctx context.Context) error {
			rec.Get(t).Record(ctx)
			return nil
		}
	})

	subject := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.Execute{Participant: pid.Get(t)}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Then("the participant sees its ID as its own path segment", func(t *testcase.T) {
			assert.NoError(t, act(t))

			paths := rec.Get(t).Snapshot()
			assert.Equal(t, 1, len(paths))
			assert.Equal(t, pathOf(pid.Get(t).String()), paths[0])
		})
	})
}

func TestSequenceAddsSegmentAndPerIterationIndex(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	rec := let.Var(s, func(t *testcase.T) *pathRecorder {
		return &pathRecorder{}
	})

	children := []testcase.Var[workflow.ParticipantID]{
		wftest.LetParticipantID(s),
		wftest.LetParticipantID(s),
		wftest.LetParticipantID(s),
	}
	for _, cp := range children {
		cp := cp
		wftest.LetParticipantWithID(s, cp, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				rec.Get(t).Record(ctx)
				return nil
			}
		})
	}

	subject := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.Execute{Participant: children[0].Get(t)},
			workflow.Execute{Participant: children[1].Get(t)},
			workflow.Execute{Participant: children[2].Get(t)},
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Then("each iteration sees its own [i] segment, in order, with no leakage", func(t *testcase.T) {
			assert.NoError(t, act(t))

			paths := rec.Get(t).Snapshot()
			assert.Equal(t, 3, len(paths))
			for i, p := range paths {
				expected := pathOf("sequence", "["+strconv.Itoa(i)+"]", children[i].Get(t).String())
				assert.Equal(t, expected, p,
					assert.MessageF("iteration %d should see only its own segment", i))
			}
		})
	})
}

func TestSequencePerIterationDoesNotLeakBetweenIterations(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	rec := let.Var(s, func(t *testcase.T) *pathRecorder {
		return &pathRecorder{}
	})

	children := []testcase.Var[workflow.ParticipantID]{
		wftest.LetParticipantID(s),
		wftest.LetParticipantID(s),
	}
	for _, cp := range children {
		cp := cp
		wftest.LetParticipantWithID(s, cp, func(t *testcase.T) func(context.Context) error {
			return func(ctx context.Context) error {
				rec.Get(t).Record(ctx)
				return nil
			}
		})
	}

	subject := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.Execute{Participant: children[0].Get(t)},
			workflow.Execute{Participant: children[1].Get(t)},
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Then("iteration [1] does not inherit the [0] segment from iteration [0]", func(t *testcase.T) {
			assert.NoError(t, act(t))

			paths := rec.Get(t).Snapshot()
			assert.Equal(t, 2, len(paths))

			assert.Equal(t, pathOf("sequence", "[0]", children[0].Get(t).String()), paths[0])
			assert.Equal(t, pathOf("sequence", "[1]", children[1].Get(t).String()), paths[1])
			assert.NotContains(t, paths[1], workflow.Scope{Name: "[0]"},
				assert.MessageF("iteration [1] must not inherit the [0] segment from iteration [0]"),
			)
		})
	})
}

func TestSequenceDoesNotMutateCallerContext(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	// Capture the caller's path before and after running an empty Sequence.
	// The path must be identical: Sequence.Execute pushes its "sequence"
	// segment only into the new contexts it hands to its children, never
	// into the caller's ctx.
	base := let.Var(s, func(t *testcase.T) context.Context {
		return workflow.WithName(t.Context(), "caller")
	})

	before := let.Act[workflow.Path](func(t *testcase.T) workflow.Path {
		return workflow.CurrentPath(base.Get(t))
	})

	subject := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		after := let.Act[workflow.Path](func(t *testcase.T) workflow.Path {
			assert.NoError(t, act(t))
			return workflow.CurrentPath(base.Get(t))
		})

		s.Then("the caller's context path is unchanged by Sequence.Execute", func(t *testcase.T) {
			assert.Equal(t, before(t), after(t))
		})
	})
}

func TestIfTrueBranchPropagatesIfThenSegments(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	rec := let.Var(s, func(t *testcase.T) *pathRecorder {
		return &pathRecorder{}
	})

	_, pid := wftest.LetParticipant[func(context.Context) error](s, func(t *testcase.T) func(context.Context) error {
		return func(ctx context.Context) error {
			rec.Get(t).Record(ctx)
			return nil
		}
	})

	subject := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.If{
			Cond: wftemplate.Condition("true"),
			Then: workflow.Execute{Participant: pid.Get(t)},
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Then("Then sees its [if, then, ...] path", func(t *testcase.T) {
			assert.NoError(t, act(t))

			paths := rec.Get(t).Snapshot()
			assert.Equal(t, 1, len(paths))
			assert.Equal(t, pathOf("if", "then", pid.Get(t).String()), paths[0])
		})
	})
}

func TestIfFalseBranchPropagatesIfElseSegments(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	rec := let.Var(s, func(t *testcase.T) *pathRecorder {
		return &pathRecorder{}
	})

	_, participantID := wftest.LetParticipant[func(context.Context) error](s, func(t *testcase.T) func(context.Context) error {
		return func(ctx context.Context) error {
			rec.Get(t).Record(ctx)
			return nil
		}
	})

	subject := let.Var(s, func(t *testcase.T) workflow.Definition {
		return workflow.If{
			Cond: wftemplate.Condition("false"),
			Else: workflow.Execute{Participant: participantID.Get(t)},
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			Context   = c.LetContext(s)
			processID = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(Context.Get(t), processID.Get(t))
		})

		s.Then("Else sees its [if, else, ...] path", func(t *testcase.T) {
			assert.NoError(t, act(t))

			paths := rec.Get(t).Snapshot()
			assert.Equal(t, 1, len(paths))
			assert.Equal(t, pathOf("if", "else", participantID.Get(t).String()), paths[0])
		})
	})
}

func TestPath(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) workflow.Path {
		return letRandomPath(t, t.Random.IntBetween(0, 7))
	})

	s.Describe("#Equal", func(s *testcase.Spec) {
		var (
			oth = let.Var[workflow.Path](s, nil)
		)
		act := let.Act(func(t *testcase.T) bool {
			return subject.Get(t).Equal(oth.Get(t))
		})

		s.Before(func(t *testcase.T) {
			t.LogPretty("subject", subject.Get(t))
			t.LogPretty("other", oth.Get(t))
		})

		s.When("paths are equal", func(s *testcase.Spec) {
			oth.Let(s, subject.Get)

			s.Then("equality is true", func(t *testcase.T) {
				assert.True(t, act(t))
			})

			s.Context("because both is empty", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) workflow.Path {
					if t.Random.Bool() {
						return nil
					}
					return workflow.Path{}
				})
				oth.Let(s, func(t *testcase.T) workflow.Path {
					if t.Random.Bool() {
						return nil
					}
					return workflow.Path{}
				})

				s.Then("they are equal", func(t *testcase.T) {
					assert.True(t, act(t))
				})
			})
		})

		s.When("paths are same length but not equal values", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.Path {
				return letRandomPath(t, t.Random.IntBetween(3, 7))
			})

			oth.Let(s, func(t *testcase.T) workflow.Path {
				var p workflow.Path = make(workflow.Path, len(subject.Get(t)))
				for i, v := range subject.Get(t) {
					p[i] = workflow.Scope{Name: random.Unique(t.Random.String, v.Name), Var: v.Var}
				}
				return p
			})

			s.Then("they are not equal", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})

		s.When("paths only differ in which of their scopes are variable scopes", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.Path {
				return letRandomPath(t, t.Random.IntBetween(1, 7))
			})
			oth.Let(s, func(t *testcase.T) workflow.Path {
				var p = slices.Clone(subject.Get(t))
				var i = t.Random.IntN(len(p))
				p[i].Var = !p[i].Var
				return p
			})

			s.Then("they are equal, as they are the same location", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("path is longer than other path, but the prefix is the same", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.Path {
				return letRandomPath(t, t.Random.IntBetween(3, 7))
			})
			oth.Let(s, func(t *testcase.T) workflow.Path {
				return subject.Get(t)[0 : len(subject.Get(t))-1]
			})
			s.Then("they are not equal", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})

		s.When("other path is longer than subject path, but the prefix is the same", func(s *testcase.Spec) {
			oth.Let(s, func(t *testcase.T) workflow.Path {
				return letRandomPath(t, t.Random.IntBetween(3, 7))
			})
			subject.Let(s, func(t *testcase.T) workflow.Path {
				return oth.Get(t)[0 : len(oth.Get(t))-1]
			})
			s.Then("they are not equal", func(t *testcase.T) {
				assert.False(t, act(t))
			})
		})
	})

	s.Describe("#VarScope", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) workflow.Path {
			return subject.Get(t).VarScope()
		})

		s.Before(func(t *testcase.T) {
			t.OnFail(func() { t.LogPretty("subject", subject.Get(t)) })
		})

		s.Then("it is a prefix of the path", func(t *testcase.T) {
			assert.True(t, subject.Get(t).MatchPrefix(act(t)))
		})

		s.Then("it is either the root scope, or it ends in a variable scope", func(t *testcase.T) {
			if got := act(t); 0 < len(got) {
				assert.True(t, got[len(got)-1].Var)
			}
		})

		s.When("the path has no variable scope", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.Path {
				return pathOf(random.Slice(t.Random.IntBetween(0, 7), t.Random.String)...)
			})

			s.Then("it is the root variable scope", func(t *testcase.T) {
				assert.Empty(t, act(t))
			})
		})

		s.When("the path ends in a variable scope", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.Path {
				return append(letRandomPath(t, t.Random.IntBetween(0, 5)), workflow.Scope{Name: t.Random.String(), Var: true})
			})

			s.Then("it is the path itself", func(t *testcase.T) {
				assertSamePath(t, subject.Get(t), act(t))
			})
		})

		s.When("the path continues after its innermost variable scope", func(s *testcase.Spec) {
			scope := let.Var(s, func(t *testcase.T) workflow.Path {
				return append(letRandomPath(t, t.Random.IntBetween(0, 5)), workflow.Scope{Name: t.Random.String(), Var: true})
			})
			subject.Let(s, func(t *testcase.T) workflow.Path {
				return slices.Concat(scope.Get(t), pathOf(random.Slice(t.Random.IntBetween(1, 5), t.Random.String)...))
			})

			s.Then("it is the path up to and including its innermost variable scope", func(t *testcase.T) {
				assertSamePath(t, scope.Get(t), act(t))
			})

			s.Then("extending it leaves the path intact", func(t *testcase.T) {
				var exp = slices.Clone(subject.Get(t))
				_ = append(act(t), workflow.Scope{Name: t.Random.String()})
				assertSamePath(t, exp, subject.Get(t))
			})
		})
	})
}

func TestWithVarScope(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		Context = let.Var(s, func(t *testcase.T) context.Context {
			return withPath(t.Context(), letRandomPath(t, t.Random.IntBetween(0, 5)))
		})
		name = let.String(s)
	)
	act := let.Act(func(t *testcase.T) context.Context {
		return workflow.WithVarScope(Context.Get(t), name.Get(t))
	})

	s.Then("it extends the current path with a variable scope of the given name", func(t *testcase.T) {
		var exp = append(workflow.CurrentPath(Context.Get(t)), workflow.Scope{Name: name.Get(t), Var: true})
		assertSamePath(t, exp, workflow.CurrentPath(act(t)))
	})

	s.Then("the new scope is the innermost variable scope", func(t *testcase.T) {
		var exp = append(workflow.CurrentPath(Context.Get(t)), workflow.Scope{Name: name.Get(t), Var: true})
		assertSamePath(t, exp, workflow.CurrentPath(act(t)).VarScope())
	})

	s.Then("the parent context keeps its own path", func(t *testcase.T) {
		var exp = workflow.CurrentPath(Context.Get(t))
		_ = act(t)
		assertSamePath(t, exp, workflow.CurrentPath(Context.Get(t)))
	})

	s.When("the context is nil", func(s *testcase.Spec) {
		Context.LetValue(s, nil)

		s.Then("the variable scope is the only scope of the path", func(t *testcase.T) {
			assertSamePath(t, varScopeOf(name.Get(t)), workflow.CurrentPath(act(t)))
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		var (
			ctx0 = context.Background()
			ctx1 = workflow.WithVarScope(ctx0, "foo")
			ctx2 = workflow.WithName(ctx1, "bar")
			ctx3 = workflow.WithVarScope(ctx2, "baz")
			ctx4 = workflow.WithName(ctx3, "qux")
		)

		assertSamePath(t, workflow.CurrentPath(ctx4), workflow.Path{
			{Name: "foo", Var: true},
			{Name: "bar"},
			{Name: "baz", Var: true},
			{Name: "qux"},
		})
		assert.Equal(t, workflow.CurrentPath(ctx4).VarScope(), pathOf("foo", "bar", "baz"))
		assert.Equal(t, workflow.CurrentPath(ctx3).VarScope(), pathOf("foo", "bar", "baz"))
		assert.Equal(t, workflow.CurrentPath(ctx2).VarScope(), pathOf("foo"))
		assert.Equal(t, workflow.CurrentPath(ctx1).VarScope(), pathOf("foo"))
		assert.Empty(t, workflow.CurrentPath(ctx0).VarScope())
	})
}
