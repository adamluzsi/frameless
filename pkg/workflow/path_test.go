package workflow_test

import (
	"context"
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
	_ = workflow.CurrentPath(ctx) // workflow.Path{"root", "child"}
}

// pathOf builds a workflow.Path out of plain scope names.
func pathOf(names ...string) workflow.Path {
	return workflow.Path(names)
}

// letRandomPath returns a random Path of the given length, with unique scope names.
func letRandomPath(t *testcase.T, length int) workflow.Path {
	return random.Slice(length, func() string {
		return t.Random.String()
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
				return random.Slice(t.Random.IntBetween(1, 7), func() string {
					return t.Random.String()
				})
			})

			Levels := let.Var(s, func(t *testcase.T) []context.Context {
				var levels = make([]context.Context, len(expectedPath.Get(t)))
				var ctx = context.Background()
				for i, name := range expectedPath.Get(t) {
					ctx = workflow.WithName(ctx, name)
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
				assert.Equal(t, expectedPath.Get(t), act(t))
			})

			s.Then("each context level keeps its own state intact", func(t *testcase.T) {
				for i := range expectedPath.Get(t) {
					var currentPath = workflow.CurrentPath(Levels.Get(t)[i])
					var expectedPath = expectedPath.Get(t)[:i+1]
					assert.Equal(t, expectedPath, currentPath)
				}
			})
		})
		s.Context("contains a variable scope made with workflow.WithVarScope", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := workflow.WithName(t.Context(), "sequence")
				ctx = workflow.WithVarScope(ctx, "loop")
				return workflow.WithName(ctx, "participant")
			})

			s.Then("the variable scope is not part of the execution path", func(t *testcase.T) {
				assert.Equal(t, pathOf("sequence", "participant"), act(t),
					assert.MessageF("the variable scope lives on its own chain, separate from the execution path"))
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

			assert.Equal(t, workflow.CurrentPath(ctx3), workflow.Path{"foo", "bar", "baz"})
			assert.Equal(t, workflow.CurrentPath(ctx2), workflow.Path{"foo", "bar"})
			assert.Equal(t, workflow.CurrentPath(ctx1), workflow.Path{"foo"})
			assert.Equal(t, workflow.CurrentPath(ctx2B), workflow.Path{"foo", "bar-2"})
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

func TestExecuteParticipantAddsItsSegment(t *testing.T) {
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
		return workflow.ExecuteParticipant{ID: pid.Get(t)}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)
		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Then("the participant sees its own path segment", func(t *testcase.T) {
			assert.NoError(t, act(t))

			paths := rec.Get(t).Snapshot()
			assert.Equal(t, 1, len(paths))
			assert.Equal(t, pathOf("participant", pid.Get(t).String()), paths[0])
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
			workflow.ExecuteParticipant{ID: children[0].Get(t)},
			workflow.ExecuteParticipant{ID: children[1].Get(t)},
			workflow.ExecuteParticipant{ID: children[2].Get(t)},
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
				expected := pathOf("sequence", "["+strconv.Itoa(i)+"]", "participant", children[i].Get(t).String())
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
			workflow.ExecuteParticipant{ID: children[0].Get(t)},
			workflow.ExecuteParticipant{ID: children[1].Get(t)},
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

			assert.Equal(t, pathOf("sequence", "[0]", "participant", children[0].Get(t).String()), paths[0])
			assert.Equal(t, pathOf("sequence", "[1]", "participant", children[1].Get(t).String()), paths[1])
			assert.NotContains(t, paths[1], "[0]",
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
			Then: workflow.ExecuteParticipant{ID: pid.Get(t)},
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
			assert.Equal(t, pathOf("if", "then", "participant", pid.Get(t).String()), paths[0])
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
			Else: workflow.ExecuteParticipant{ID: participantID.Get(t)},
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
			assert.Equal(t, pathOf("if", "else", "participant", participantID.Get(t).String()), paths[0])
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
					p[i] = random.Unique(t.Random.String, v)
				}
				return p
			})

			s.Then("they are not equal", func(t *testcase.T) {
				assert.False(t, act(t))
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
}

func TestCurrentVarScope(t *testing.T) {
	s := testcase.NewSpec(t)

	var Context = let.Var(s, func(t *testcase.T) context.Context {
		return t.Context()
	})
	act := let.Act(func(t *testcase.T) workflow.VarScope {
		return workflow.CurrentVarScope(Context.Get(t))
	})

	s.Then("it returns a VarScope without an issue", func(t *testcase.T) {
		var _ workflow.VarScope = act(t)
	})

	s.Context("when the context", func(s *testcase.Spec) {
		s.Context("is nil", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				return nil
			})

			s.Then("it returns an empty variable scope", func(t *testcase.T) {
				assert.Empty(t, act(t))
			})
		})

		s.Context("has no variable scope entries", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				return context.Background()
			})

			s.Then("it returns an empty variable scope", func(t *testcase.T) {
				assert.Empty(t, act(t))
			})
		})

		s.Context("contains a variable scope built with workflow.WithVarScope", func(s *testcase.Spec) {
			expectedScope := let.Var(s, func(t *testcase.T) workflow.VarScope {
				return random.Slice(t.Random.IntBetween(1, 7), func() string {
					return t.Random.String()
				})
			})

			Levels := let.Var(s, func(t *testcase.T) []context.Context {
				var levels = make([]context.Context, len(expectedScope.Get(t)))
				var ctx = context.Background()
				for i, name := range expectedScope.Get(t) {
					ctx = workflow.WithVarScope(ctx, name)
					levels[i] = ctx
				}
				return levels
			})

			Context.Let(s, func(t *testcase.T) context.Context {
				last, ok := slicekit.Last(Levels.Get(t))
				assert.True(t, ok)
				return last
			})

			s.Then("it returns the current variable scope", func(t *testcase.T) {
				assert.Equal(t, expectedScope.Get(t), act(t))
			})

			s.Then("each context level keeps its own state intact", func(t *testcase.T) {
				for i := range expectedScope.Get(t) {
					var currentScope = workflow.CurrentVarScope(Levels.Get(t)[i])
					var expectedScope = expectedScope.Get(t)[:i+1]
					assert.Equal(t, expectedScope, currentScope)
				}
			})
		})

		s.Context("contains execution path segments made with workflow.WithName", func(s *testcase.Spec) {
			Context.Let(s, func(t *testcase.T) context.Context {
				ctx := workflow.WithName(t.Context(), "sequence")
				ctx = workflow.WithName(ctx, "[0]")
				return workflow.WithName(ctx, "participant")
			})

			s.Then("the execution path does not leak into the variable scope", func(t *testcase.T) {
				assert.Empty(t, act(t),
					assert.MessageF("execution path segments live on their own chain, separate from the variable scope"))
			})
		})
	})

	s.Context("smoke", func(s *testcase.Spec) {
		s.Test("context based level", func(t *testcase.T) {
			var (
				ctx0  = context.Background()
				ctx1  = workflow.WithVarScope(ctx0, "foo")
				ctx2  = workflow.WithVarScope(ctx1, "bar")
				ctx2B = workflow.WithVarScope(ctx1, "bar-2")
				ctx3  = workflow.WithVarScope(ctx2, "baz")
			)

			assert.Equal(t, workflow.CurrentVarScope(ctx3), workflow.VarScope{"foo", "bar", "baz"})
			assert.Equal(t, workflow.CurrentVarScope(ctx2), workflow.VarScope{"foo", "bar"})
			assert.Equal(t, workflow.CurrentVarScope(ctx1), workflow.VarScope{"foo"})
			assert.Equal(t, workflow.CurrentVarScope(ctx2B), workflow.VarScope{"foo", "bar-2"})
			assert.Empty(t, workflow.CurrentVarScope(ctx0))
		})
	})
}
