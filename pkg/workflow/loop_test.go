package workflow_test

import (
	"context"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/frameless/pkg/workflow/wftest"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/pp"
	"go.llib.dev/testcase/random"
)

func ExampleFor() {
	var foo, bar, baz func()

	// workflow for loop
	_ = workflow.For{
		// intentionally don't use named initialization,
		// to force the setup to use similar field ordering as the for loop has.
		workflow.SetVar{Name: "i", Value: 0}, wftemplate.Condition("lt .i 5"), workflow.Increment{Name: "i"},
		workflow.Sequence{
			workflow.Execute{Participant: "foo"},
			workflow.Execute{Participant: "bar"},
			workflow.Execute{Participant: "baz"},
		},
	}

	// go for loop
	for i := 0; i < 5; i++ {
		foo()
		bar()
		baz()
	}
}

// TestFor specifies workflow.For, the workflow equivalent of Go's three clause
// for loop:
//
//	for Init; Cond; Post { Do }
//
// Init, Cond, Post and Do share a single variable scope belonging to the loop,
// so the loop variable is visible to every part of the loop without leaking out
// of it. Each round is a step of its own, so an idempotent participant in the
// body runs again on the next round rather than being skipped as already done.
func TestFor(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	// varsOf reads a Process's variables as they are visible from a context.
	var varsOf = func(t *testcase.T, pid workflow.ProcessID) workflow.Vars {
		return workflow.Vars{
			ProcessID:        pid,
			EventsRepository: c.EventRepository.Get(t),
		}
	}

	var (
		// counter is the loop variable: the "i" of `for i := 0; i < n; i++`.
		counter = let.VarOf[workflow.VarName](s, "i")

		// rounds is how many times the condition lets the body run.
		rounds = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntBetween(2, 5)
		})

		// visited records the loop variable as the body saw it, one entry per
		// round, in the order the rounds happened.
		visited = let.VarOf[[]any](s, nil)

		// initRuns counts how many times the loop's init step ran.
		initRuns = let.VarOf(s, 0)

		// condEvaluations counts how many times the condition was asked.
		condEvaluations = let.VarOf(s, 0)
	)

	var (
		init = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.DeclareVar{Name: counter.Get(t)},
				workflow.SetVar{Name: counter.Get(t), Value: 0},
				wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						initRuns.Set(t, initRuns.Get(t)+1)
						return nil
					},
				},
			}
		})

		cond = let.Var[workflow.Condition](s, func(t *testcase.T) workflow.Condition {
			return wftest.Stub{
				StubEvaluate: func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
					v, ok, err := varsOf(t, pid).Lookup(ctx, counter.Get(t))
					if err != nil {
						return false, err
					}
					if !ok {
						return false, fmt.Errorf("the loop variable %q is not visible to the condition", counter.Get(t))
					}
					n, ok := v.(int)
					if !ok {
						return false, fmt.Errorf("the loop variable is a %T, not an int", v)
					}
					return n < rounds.Get(t), nil
				},
			}
		})

		post = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
			return workflow.Increment{Name: counter.Get(t)}
		})

		do = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
			return wftest.Stub{
				StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
					v, _, err := varsOf(t, pid).Lookup(ctx, counter.Get(t))
					if err != nil {
						return err
					}
					testcase.Append(t, visited, v)
					return nil
				},
			}
		})
	)

	// maxRounds is the ceiling past which a loop is treated as non-terminating.
	//
	// A loop which fails to end must fail the test rather than hang the suite.
	// The ceiling belongs to the loop under test rather than to any one of its
	// parts, so both of the guards below carry it: `bounded` for the loops which
	// end on their condition, and `breakAfterRounds` for the loops which have no
	// condition and can only end on a Break.
	const maxRounds = 100

	// bounded counts how many times the loop asks its condition, and refuses to
	// answer past the ceiling, so that a test which supplies a condition of its
	// own is protected just the same as one using the default.
	var bounded = func(t *testcase.T, cond workflow.Condition) workflow.Condition {
		if cond == nil {
			return nil
		}
		return wftest.Stub{
			StubEvaluate: func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
				condEvaluations.Set(t, condEvaluations.Get(t)+1)
				if maxRounds < condEvaluations.Get(t) {
					return false, fmt.Errorf("the loop asked its condition %d times without ever terminating",
						condEvaluations.Get(t))
				}
				return cond.Evaluate(ctx, pid)
			},
		}
	}

	// breakAfterRounds raises a workflow.Break once the body has been visited as
	// many times as the loop was meant to run, and carries the ceiling for the
	// loops which have no condition to carry it for them.
	var breakAfterRounds = func(t *testcase.T) workflow.Definition {
		return wftest.Stub{
			StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
				switch visits := len(visited.Get(t)); {
				case maxRounds < visits:
					return fmt.Errorf("the loop ran %d rounds without ever breaking out", visits)
				case rounds.Get(t) <= visits:
					return workflow.Break{}
				default:
					return nil
				}
			},
		}
	}

	subject := let.Var(s, func(t *testcase.T) workflow.For {
		return workflow.For{
			Init: init.Get(t),
			Cond: bounded(t, cond.Get(t)),
			Post: post.Get(t),
			Do:   do.Get(t),
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

		// countingUpFromZero is what the body must have seen after a loop that
		// started at zero and advanced by one on every round.
		var countingUpFromZero = func(t *testcase.T) []any {
			var vs []any
			for i := 0; i < rounds.Get(t); i++ {
				vs = append(vs, i)
			}
			return vs
		}

		s.Then("the body runs once for every round the condition allows", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, rounds.Get(t), len(visited.Get(t)))
		})

		s.Then("the body sees the loop variable count up from zero", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, countingUpFromZero(t), visited.Get(t),
				"the post step advances the loop variable after the body ran, not before")
		})

		s.Then("the init step runs exactly once", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, 1, initRuns.Get(t),
				"the loop is set up once, not once per round")
		})

		s.Then("the loop variable is not visible after the loop is over", func(t *testcase.T) {
			assert.NoError(t, act(t))

			_, ok, err := varsOf(t, process.Get(t)).Lookup(ctx.Get(t), counter.Get(t))
			assert.NoError(t, err)
			assert.False(t, ok,
				"the loop variable belongs to the loop's own variable scope")
		})

		s.When("the condition is false from the very start", func(s *testcase.Spec) {
			rounds.LetValue(s, 0)

			s.Then("the body never runs", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Empty(t, visited.Get(t))
			})

			s.Then("the init step still runs", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, 1, initRuns.Get(t),
					"a loop that runs zero times is still a loop that was set up")
			})
		})

		s.When("the loop has no body", func(s *testcase.Spec) {
			do.Let(s, func(t *testcase.T) workflow.Definition { return nil })

			s.Then("the loop still runs its rounds and finishes", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, rounds.Get(t)+1, condEvaluations.Get(t),
					"the condition is checked before every round, plus once to end the loop")
			})
		})

		s.When("the loop has no post step", func(s *testcase.Spec) {
			post.Let(s, func(t *testcase.T) workflow.Definition { return nil })

			// Without a post step the body has to advance the loop variable
			// itself, the same way a Go for loop with an empty post clause does.
			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					do.Super(t),
					workflow.Increment{Name: counter.Get(t)},
				}
			})

			s.Then("the loop keeps running until the condition is no longer met", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, countingUpFromZero(t), visited.Get(t))
			})
		})

		s.When("the loop has no init step", func(s *testcase.Spec) {
			init.Let(s, func(t *testcase.T) workflow.Definition { return nil })

			// With nothing to initialise, the loop counts a variable that was
			// already declared outside of it.
			s.Before(func(t *testcase.T) {
				assert.NoError(t, varsOf(t, process.Get(t)).
					Set(ctx.Get(t), counter.Get(t), 0))
			})

			s.Then("the loop counts the variable it was given", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, countingUpFromZero(t), visited.Get(t))
			})

			s.Then("the outer variable keeps the value the loop counted it to", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok, err := varsOf(t, process.Get(t)).Lookup(ctx.Get(t), counter.Get(t))
				assert.NoError(t, err)
				assert.True(t, ok, "a variable from outside the loop outlives it")
				assert.Equal[any](t, got, rounds.Get(t))
			})
		})

		s.When("the loop has no condition", func(s *testcase.Spec) {
			cond.Let(s, func(t *testcase.T) workflow.Condition { return nil })

			// A loop with no condition is `for { ... }`: nothing in the loop
			// clause will ever end it, so the body has to break out of it.
			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					do.Super(t),
					breakAfterRounds(t),
				}
			})

			s.Then("the loop keeps running until something breaks out of it", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, countingUpFromZero(t), visited.Get(t))
			})
		})

		s.When("the body breaks out of the loop", func(s *testcase.Spec) {
			// breakAt is the round the body breaks at, chosen before the
			// condition would have ended the loop on its own.
			var breakAt = let.Var(s, func(t *testcase.T) int {
				return t.Random.IntBetween(1, rounds.Get(t)-1)
			})

			var postRuns = let.VarOf(s, 0)

			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					do.Super(t),
					wftest.Stub{
						StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
							if breakAt.Get(t) <= len(visited.Get(t)) {
								return workflow.Break{}
							}
							return nil
						},
					},
				}
			})

			post.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					post.Super(t),
					wftest.Stub{
						StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
							postRuns.Set(t, postRuns.Get(t)+1)
							return nil
						},
					},
				}
			})

			s.Then("the loop ends there, earlier than its condition would have ended it", func(t *testcase.T) {
				assert.NoError(t, act(t),
					"breaking out of a loop is how a loop ends, not a failure to report upwards")

				assert.Equal(t, breakAt.Get(t), len(visited.Get(t)))
			})

			s.Then("the round which breaks does not run its post step", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, breakAt.Get(t)-1, postRuns.Get(t),
					"a break leaves the round at once, the way Go's break skips the post statement")
			})
		})

		s.When("the post step breaks out of the loop", func(s *testcase.Spec) {
			post.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Break{}
			})

			s.Then("the loop ends after the body's first round", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, 1, len(visited.Get(t)))
			})
		})

		s.When("the init step breaks out of the loop", func(s *testcase.Spec) {
			init.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Break{}
			})

			s.Then("no round ever runs", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Empty(t, visited.Get(t))
				assert.Equal(t, 0, condEvaluations.Get(t),
					"a loop which was broken out of before it began has nothing to evaluate")
			})
		})

		s.When("the condition is written as a template expression", func(s *testcase.Spec) {
			cond.Let(s, func(t *testcase.T) workflow.Condition {
				return wftemplate.Condition(fmt.Sprintf("lt .%s %d", counter.Get(t), rounds.Get(t)))
			})

			s.Then("it reads the loop variable out of the loop's own scope", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, countingUpFromZero(t), visited.Get(t))
			})
		})

		s.When("the init step fails", func(s *testcase.Spec) {
			var expErr = let.Error(s)

			init.Let(s, func(t *testcase.T) workflow.Definition {
				return wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						return expErr.Get(t)
					},
				}
			})

			s.Then("the error is returned and the loop never starts", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))

				assert.Empty(t, visited.Get(t))
				assert.Equal(t, 0, condEvaluations.Get(t),
					"a loop whose setup failed has nothing to evaluate")
			})
		})

		s.When("the condition fails", func(s *testcase.Spec) {
			var expErr = let.Error(s)

			cond.Let(s, func(t *testcase.T) workflow.Condition {
				return wftest.Stub{
					StubEvaluate: func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
						return false, expErr.Get(t)
					},
				}
			})

			s.Then("the error is returned and the body never runs", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))

				assert.Empty(t, visited.Get(t))
			})
		})

		s.When("the body fails", func(s *testcase.Spec) {
			var expErr = let.Error(s)

			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					do.Super(t),
					wftest.Stub{
						StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
							return expErr.Get(t)
						},
					},
				}
			})

			s.Then("the error is returned and the loop stops at that round", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))

				assert.Equal(t, 1, len(visited.Get(t)),
					"the round that failed is the last round")
			})
		})

		s.When("the post step fails", func(s *testcase.Spec) {
			var expErr = let.Error(s)

			post.Let(s, func(t *testcase.T) workflow.Definition {
				return wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						return expErr.Get(t)
					},
				}
			})

			s.Then("the error is returned after the body already ran once", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))

				assert.Equal(t, 1, len(visited.Get(t)))
			})
		})
	})

	s.Context("as part of a Process", func(s *testcase.Spec) {
		var (
			participantID    = wftest.LetParticipantID(s)
			participantCalls = let.VarOf(s, 0)
		)

		_ = wftest.LetParticipantWithID(s, participantID,
			func(t *testcase.T) func(context.Context) error {
				return func(ctx context.Context) error {
					participantCalls.Set(t, participantCalls.Get(t)+1)
					if maxRounds < participantCalls.Get(t) {
						return fmt.Errorf("the loop ran %d rounds without ever ending", participantCalls.Get(t))
					}
					return nil
				}
			})

		var definition = let.Var(s, func(t *testcase.T) workflow.Definition {
			return workflow.For{
				Init: workflow.Sequence{
					workflow.DeclareVar{Name: counter.Get(t)},
					workflow.SetVar{Name: counter.Get(t), Value: 0},
				},
				Cond: bounded(t, wftemplate.Condition(fmt.Sprintf("lt .%s %d", counter.Get(t), rounds.Get(t)))),
				Post: workflow.Increment{Name: counter.Get(t)},
				Do:   workflow.Execute{Participant: participantID.Get(t)},
			}
		})

		var boundProcess = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			pid := wftest.MakeProcessID(t)
			assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), pid, definition.Get(t)))
			return pid
		})

		s.Test("the participant in the loop body is executed once per round", func(t *testcase.T) {
			assert.NoError(t, c.Runtime.Get(t).Execute(t.Context(), boundProcess.Get(t)))

			assert.Equal(t, rounds.Get(t), participantCalls.Get(t),
				"every round is a step of its own,",
				"so an idempotent participant is not skipped as already executed")
		})

		s.Test("re-executing the Process replays the loop without calling the participant again", func(t *testcase.T) {
			rt := c.Runtime.Get(t)

			assert.NoError(t, rt.Execute(t.Context(), boundProcess.Get(t)))
			assert.NoError(t, rt.Execute(t.Context(), boundProcess.Get(t)))

			assert.Equal(t, rounds.Get(t), participantCalls.Get(t),
				"a replay arrives at the same state,",
				"it does not do the work a second time")
		})

		s.When("the loop has no condition and the body breaks out of it", func(s *testcase.Spec) {
			definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.For{
					Init: workflow.Sequence{
						workflow.DeclareVar{Name: counter.Get(t)},
						workflow.SetVar{Name: counter.Get(t), Value: 0},
					},
					Post: workflow.Increment{Name: counter.Get(t)},
					Do: workflow.Sequence{
						workflow.Execute{Participant: participantID.Get(t)},
						workflow.If{
							Cond: wftemplate.Condition(fmt.Sprintf("le %d .%s", rounds.Get(t)-1, counter.Get(t))),
							Then: workflow.Break{},
						},
					},
				}
			})

			s.Then("the participant is executed once per round, up to the round which breaks", func(t *testcase.T) {
				assert.NoError(t, c.Runtime.Get(t).Execute(t.Context(), boundProcess.Get(t)))

				assert.Equal(t, rounds.Get(t), participantCalls.Get(t))
			})

			s.Then("the work done before the break is kept, so a replay does not repeat it", func(t *testcase.T) {
				rt := c.Runtime.Get(t)

				assert.NoError(t, rt.Execute(t.Context(), boundProcess.Get(t)))
				assert.NoError(t, rt.Execute(t.Context(), boundProcess.Get(t)))

				assert.Equal(t, rounds.Get(t), participantCalls.Get(t),
					"breaking out of a loop is not a failure,",
					"so the rounds it leaves behind stay recorded rather than being rolled back")
			})
		})

		s.When("the body changes the variable its condition reads, and then suspends", func(s *testcase.Spec) {
			suspendCalls := let.VarOf(s, 0)
			wftest.LetParticipantWithID(s, let.VarOf(s, workflow.ParticipantID("suspend-once")), func(t *testcase.T) func(context.Context) error {
				return func(context.Context) error {
					suspendCalls.Set(t, suspendCalls.Get(t)+1)
					if suspendCalls.Get(t) == 1 {
						return workflow.Suspend{}
					}
					return nil
				}
			})
			afterCalls := let.VarOf(s, 0)
			wftest.LetParticipantWithID(s, let.VarOf(s, workflow.ParticipantID("after")), func(t *testcase.T) func(context.Context) error {
				return func(context.Context) error {
					afterCalls.Set(t, afterCalls.Get(t)+1)
					return nil
				}
			})
			definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.For{
					Init: workflow.Sequence{
						workflow.DeclareVar{Name: "done"},
						workflow.SetVar{Name: "done", Value: false},
					},
					Cond: bounded(t, wftemplate.Condition("not .done")),
					Do: workflow.Sequence{
						workflow.Execute{Participant: participantID.Get(t)},
						workflow.SetVar{Name: "done", Value: true},
						workflow.Execute{Participant: "suspend-once"},
						workflow.Execute{Participant: "after"},
					},
				}
			})

			s.Then("the resumed round finishes its body, since the condition's answer for the round is replayed", func(t *testcase.T) {
				rt := c.Runtime.Get(t)

				assert.ErrorIs(t, rt.Execute(t.Context(), boundProcess.Get(t)), workflow.Suspend{})
				assert.NoError(t, rt.Execute(t.Context(), boundProcess.Get(t)))

				assert.Equal(t, participantCalls.Get(t), 1)
				assert.Equal(t, suspendCalls.Get(t), 2)
				assert.Equal(t, afterCalls.Get(t), 1,
					"the round was already running when done changed,",
					"so only the next round's condition may see it")
			})
		})
	})

	// The variables of a For are scoped the way Go scopes them:
	//
	//	for i := 0; i < n; i++ { // i belongs to the loop
	//		x := i               // x belongs to the body of the round
	//	}
	//
	// The loop has a variable scope of its own, which Init, Cond and Post share,
	// and the body gets a fresh scope nested in it on every round.
	s.Context("var-scope", func(s *testcase.Spec) {
		var seen = let.VarOf[[]any](s, nil)

		s.Test("a variable declared in the body is fresh on every round", func(t *testcase.T) {
			// for i := 0; i < 3; i++ {
			// 	see(x) // undeclared on every round
			// 	x := "declared-in-body"
			// }
			_, err := execProcess(t, c.Runtime.Get(t), workflow.For{
				Init: workflow.SetVar{Name: "i", Value: 0},
				Cond: wftemplate.Condition("lt .i 3"),
				Post: workflow.Increment{Name: "i"},
				Do: workflow.Sequence{
					peekVar(t, seen, "x"),
					workflow.SetVar{Name: "x", Value: "declared-in-body"},
				},
			})
			assert.NoError(t, err)

			assert.Equal(t, []any{undeclared, undeclared, undeclared}, seen.Get(t),
				"a round must not start with the variables an earlier round declared in its body")
		})

		s.Test("the loop clause doesn't see the variables of the body", func(t *testcase.T) {
			// for i := 0; see(x), i < 2; see(x), i++ {
			// 	x := "declared-in-body"
			// }
			//
			_, err := execProcess(t, c.Runtime.Get(t), workflow.For{
				Init: workflow.SetVar{Name: "i", Value: 0},
				Cond: wftest.Stub{StubEvaluate: func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
					if err := peekVar(t, seen, "x").Execute(ctx, pid); err != nil {
						return false, err
					}
					return wftemplate.Condition("lt .i 2").Evaluate(ctx, pid)
				}},
				Post: workflow.Sequence{
					peekVar(t, seen, "x"),
					workflow.Increment{Name: "i"},
				},
				Do: workflow.SetVar{Name: "x", Value: "declared-in-body"},
			})
			assert.NoError(t, err)

			pp.PP(seen.Get(t))
			assert.Equal(t, []any{undeclared, undeclared, undeclared, undeclared, undeclared}, seen.Get(t),
				"Cond and Post belong to the loop, outside of the body's scope")
		})

		s.Test("an assignment in the body to a variable from outside of it writes through", func(t *testcase.T) {
			// total := 0
			// for i := 0; i < 3; i++ {
			// 	total++
			// }
			// // total == 3
			pid, err := execProcess(t, c.Runtime.Get(t), workflow.Sequence{
				workflow.SetVar{Name: "total", Value: 0},
				workflow.For{
					Init: workflow.SetVar{Name: "i", Value: 0},
					Cond: wftemplate.Condition("lt .i 3"),
					Post: workflow.Increment{Name: "i"},
					Do:   workflow.Increment{Name: "total"},
				},
			})
			assert.NoError(t, err)

			assert.Equal[any](t, 3, getVar(t, c.Runtime.Get(t), pid, "total"),
				"the body assigns to the variable it refers to, rather than declaring one of its own")
		})

		s.Test("a sibling loop doesn't see the variables of the loop before it", func(t *testcase.T) {
			// for i := 0; i < 1; i++ {
			// 	x := "declared-in-first-loop"
			// }
			// for see(i), i := 0; i < 1; i++ {
			// 	see(x)
			// }
			_, err := execProcess(t, c.Runtime.Get(t), workflow.Sequence{
				workflow.For{
					Init: workflow.SetVar{Name: "i", Value: 0},
					Cond: wftemplate.Condition("lt .i 1"),
					Post: workflow.Increment{Name: "i"},
					Do:   workflow.SetVar{Name: "x", Value: "declared-in-first-loop"},
				},
				workflow.For{
					Init: workflow.Sequence{
						peekVar(t, seen, "i"),
						workflow.SetVar{Name: "i", Value: 0},
					},
					Cond: wftemplate.Condition("lt .i 1"),
					Post: workflow.Increment{Name: "i"},
					Do:   peekVar(t, seen, "x"),
				},
			})
			assert.NoError(t, err)

			assert.Equal(t, []any{undeclared, undeclared}, seen.Get(t),
				"the first loop's variables went out of scope with the first loop")
		})

		s.Test("a Defer in the body sees the variables of its own round", func(t *testcase.T) {
			// for i := 0; i < 2; i++ {
			// 	x := i
			// 	defer func() { see(x) }()
			// }
			// // see(1), see(0)
			_, err := execProcess(t, c.Runtime.Get(t), workflow.For{
				Init: workflow.SetVar{Name: "i", Value: 0},
				Cond: wftemplate.Condition("lt .i 2"),
				Post: workflow.Increment{Name: "i"},
				Do: workflow.Sequence{
					copyVar("i", "x"),
					workflow.Defer{Definition: peekVar(t, seen, "x")},
				},
			})
			assert.NoError(t, err)

			assert.Equal(t, []any{1, 0}, seen.Get(t),
				"every round has its own x, and the Defer of the round refers to that one")
		})

		s.Context("a resumed Process replays the loop with the same variables", func(s *testcase.Spec) {
			var (
				calls     = let.VarOf[[]string](s, nil)
				suspended = let.VarOf(s, false)
			)
			c.Participants.Let(s, func(t *testcase.T) workflow.Participants {
				return workflow.Participants{
					"stringify": func(ctx context.Context, v int) (string, error) {
						return fmt.Sprint(v), nil
					},
					"see": func(ctx context.Context, v string) error {
						testcase.Append(t, calls, v)
						return nil
					},
					"suspend-once": func(ctx context.Context) error {
						if suspended.Get(t) {
							return nil
						}
						suspended.Set(t, true)
						return workflow.Suspend{}
					},
				}
			})

			s.Test("the variables of the body, on every round", func(t *testcase.T) {
				// for i := 0; i < 2; i++ {
				// 	x := stringify(i)
				// 	see(x)
				// }
				// suspend()
				rt := c.Runtime.Get(t)
				pid, err := execProcess(t, rt, workflow.Sequence{
					workflow.For{
						Init: workflow.SetVar{Name: "i", Value: 0},
						Cond: wftemplate.Condition("lt .i 2"),
						Post: workflow.Increment{Name: "i"},
						Do: workflow.Sequence{
							workflow.Execute{Participant: "stringify", Input: []workflow.VarName{"i"}, Output: []workflow.VarName{"x"}},
							workflow.Execute{Participant: "see", Input: []workflow.VarName{"x"}},
						},
					},
					workflow.Execute{Participant: "suspend-once"},
				})
				assert.Must(t).ErrorIs(err, workflow.Suspend{})
				assert.Must(t).NoError(rt.Execute(t.Context(), pid))

				assert.Equal(t, []string{"0", "1"}, calls.Get(t),
					"every round is seen exactly once, and the resumed execution replays each of them")

				_, ok, err := getVars(t, rt, pid).Lookup(t.Context(), "x")
				assert.NoError(t, err)
				assert.False(t, ok, "the variables of the body don't outlive the loop")
			})
		})
	})
}

func ExampleBreak() {
	var poll func() (done bool)

	// workflow loop which polls until the job is done
	_ = workflow.For{
		Do: workflow.Sequence{
			workflow.Execute{Participant: "poll-job", Output: []workflow.VarName{"done"}},
			workflow.If{
				Cond: wftemplate.Condition(".done"),
				Then: workflow.Break{},
			},
		},
	}

	// go loop
	for {
		done := poll()
		if done {
			break
		}
	}
}

// TestBreak specifies workflow.Break, the workflow equivalent of Go's break
// statement.
//
// Break is a Definition and nothing else: executing it raises itself, and the
// loop around it catches it. It is deliberately not a RuntimeSignal, because a
// signal is control flow the runtime re-asks on every execution and never
// records, while breaking out of a loop is decided by the Process's own
// variables, so a replay arrives at the very same break on its own.
func TestBreak(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	subject := let.Var(s, func(t *testcase.T) workflow.Break {
		return workflow.Break{}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		var (
			ctx     = c.LetContext(s)
			process = wftest.LetProcessID(s)
		)

		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).Execute(ctx.Get(t), process.Get(t))
		})

		s.Then("it raises itself, for the enclosing loop to recognise", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.Break{})
		})
	})

	s.Context("as part of a Process", func(s *testcase.Spec) {
		// rounds counts the rounds of the loop the Break is meant to end, so a
		// Break which fails to end it fails the test rather than hangs the suite.
		var rounds = let.VarOf(s, 0)

		var definition = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
			return workflow.For{ // for { break }
				Do: workflow.Sequence{
					wftest.Stub{
						StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
							rounds.Set(t, rounds.Get(t)+1)
							const maxRounds = 100
							if maxRounds < rounds.Get(t) {
								return fmt.Errorf("the loop ran %d rounds without ever ending", rounds.Get(t))
							}
							return nil
						},
					},
					workflow.Break{},
				},
			}
		})

		act := let.Act(func(t *testcase.T) error {
			return c.ActExecuteDefinition(t, t.Context(), wftest.MakeProcessID(t), definition.Get(t))
		})

		s.Then("the loop it ends completes the Process", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, 1, rounds.Get(t),
				"the loop ended on the round the Break was raised in")
		})

		s.When("the Break is not inside any loop", func(s *testcase.Spec) {
			definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{workflow.Break{}}
			})

			s.Then("the Process fails, as there is no loop for it to end", func(t *testcase.T) {
				assert.Error(t, act(t))
			})
		})
	})
}

func ExampleForEach() {
	_ = workflow.ForEach{
		Over: "orders", V: "order",
		Do: workflow.Sequence{
			workflow.Execute{Participant: "charge-order"},
			workflow.Execute{Participant: "ship-order"},
		},
	}
}

// forEachIteration is what a loop body could observe during a single iteration.
type forEachIteration struct {
	Key   any
	Value any
}

// countIterationVarEvents counts the EventDeclareVar and EventSetVar events in
// the given Process history that are recorded for the named variable.
//
// A ForEach exposes its iteration variables to the body through the execution
// context, but the spec says it must not pollute the event log with them. This
// helper counts the events we do not want to see.
func countIterationVarEvents(history []workflow.Event, name workflow.VarName) int {
	var n int
	for _, e := range history {
		switch e := e.(type) {
		case workflow.EventDeclareVar:
			if e.Name == name {
				n++
			}
		case workflow.EventSetVar:
			if e.Name == name {
				n++
			}
		}
	}
	return n
}

func TestForEach(t *testing.T) {
	s := testcase.NewSpec(t)

	var c = wftest.LetC(s)

	var (
		collectionName = let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.UUID())
		})
		keyName = let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.UUID())
		})
		valueName = let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.UUID())
		})

		// elements is the collection the loop iterates by default. A slice is
		// used, so that both the index and the element are well defined, and
		// the iteration order is fixed.
		elements = let.Var(s, func(t *testcase.T) []string {
			return random.Slice(t.Random.IntBetween(2, 5), t.Random.UUID)
		})

		collection = let.Var[any](s, func(t *testcase.T) any {
			return elements.Get(t)
		})

		// hasCollection tells whether the collection variable is present in the
		// Process at all, which is a different thing from it being empty.
		hasCollection = let.VarOf(s, true)

		// iterations records what the loop body was given on each iteration.
		iterations = let.Var(s, func(t *testcase.T) []forEachIteration {
			return nil
		})
	)

	// The loop body records the iteration variables as the body sees them.
	var do = let.Var[workflow.Definition](s, func(t *testcase.T) workflow.Definition {
		return wftest.Stub{
			StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
				var vs = workflow.Vars{
					ProcessID:        pid,
					EventsRepository: c.EventRepository.Get(t),
				}
				key, _, err := vs.Lookup(ctx, keyName.Get(t))
				if err != nil {
					return err
				}
				value, _, err := vs.Lookup(ctx, valueName.Get(t))
				if err != nil {
					return err
				}
				testcase.Append(t, iterations, forEachIteration{Key: key, Value: value})
				return nil
			},
		}
	})

	subject := let.Var(s, func(t *testcase.T) workflow.ForEach {
		return workflow.ForEach{
			Over: collectionName.Get(t),
			K:    keyName.Get(t),
			V:    valueName.Get(t),
			Do:   do.Get(t),
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

		// vars reads the Process variables the way they are seen from outside
		// of the loop, since ctx carries the variable scope the loop runs in.
		var vars = func(t *testcase.T) workflow.Vars {
			return workflow.Vars{
				ProcessID:        process.Get(t),
				EventsRepository: c.EventRepository.Get(t),
			}
		}

		// thenARepeatedExecutionSeesTheSame asserts that repeating the execution,
		// which is how a Process gets replayed, gives the loop body the very same
		// iteration variables as the first execution did.
		var thenARepeatedExecutionSeesTheSame = func(s *testcase.Spec) {
			s.Then("a repeated execution sees the same iteration variables", func(t *testcase.T) {
				assert.NoError(t, act(t))
				first := iterations.Get(t)
				iterations.Set(t, nil)

				assert.NoError(t, act(t))
				assert.Equal(t, first, iterations.Get(t),
					"what the first execution recorded must not make the loop body see different values on a replay")
			})
		}

		s.Before(func(t *testcase.T) {
			if !hasCollection.Get(t) {
				return
			}
			assert.NoError(t, vars(t).Set(ctx.Get(t), collectionName.Get(t), collection.Get(t)))
		})

		s.Then("the loop body is executed for each element of the collection", func(t *testcase.T) {
			assert.NoError(t, act(t))

			var expected []forEachIteration
			for index, element := range elements.Get(t) {
				expected = append(expected, forEachIteration{Key: index, Value: element})
			}

			assert.Equal(t, expected, iterations.Get(t),
				"the body must run once per element,",
				"seeing the index as the Key and the element as the Value")
		})

		thenARepeatedExecutionSeesTheSame(s)

		s.Then("the iteration variables don't outlive the loop", func(t *testcase.T) {
			assert.NoError(t, act(t))

			_, ok, err := vars(t).Lookup(ctx.Get(t), keyName.Get(t))
			assert.NoError(t, err)
			assert.False(t, ok, "the Key belongs to the loop body's variable scope")

			_, ok, err = vars(t).Lookup(ctx.Get(t), valueName.Get(t))
			assert.NoError(t, err)
			assert.False(t, ok, "the Value belongs to the loop body's variable scope")

			m, err := vars(t).ToMap(ctx.Get(t))
			assert.NoError(t, err)
			_, ok = m[keyName.Get(t)]
			assert.False(t, ok, "#ToMap must not see the Key either, once the loop is over")
			_, ok = m[valueName.Get(t)]
			assert.False(t, ok, "#ToMap must not see the Value either, once the loop is over")
		})

		s.Then("the iteration variables do not add events to the event log", func(t *testcase.T) {
			assert.NoError(t, act(t))

			// The iteration's Key and Value are observable to the body through
			// its execution context, but they are not persisted as variable
			// events. Each iteration is a reference on what to look up from the
			// event log later, not a state change of its own.
			history := mustHistory(t, c.Runtime.Get(t), process.Get(t))
			assert.Equal(t, 0, countIterationVarEvents(history, keyName.Get(t)),
				"the Key variable must not generate Declare/Set events")
			assert.Equal(t, 0, countIterationVarEvents(history, valueName.Get(t)),
				"the Value variable must not generate Declare/Set events")
		})

		s.Then("the collection variable is left as it was", func(t *testcase.T) {
			assert.NoError(t, act(t))

			got, ok, err := vars(t).Lookup(ctx.Get(t), collectionName.Get(t))
			assert.NoError(t, err)
			assert.True(t, ok)
			assert.Equal[any](t, collection.Get(t), got)
		})

		// Next to the single variable #Lookup, the variables can be read as a
		// whole too, which is how a template condition sees them. The loop body
		// must find its iteration variables the same way on every read path.
		s.When("the loop body reads its variables as a map", func(s *testcase.Spec) {
			do.Let(s, func(t *testcase.T) workflow.Definition {
				return wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						m, err := workflow.Vars{
							ProcessID:        pid,
							EventsRepository: c.EventRepository.Get(t),
						}.ToMap(ctx)
						if err != nil {
							return err
						}
						testcase.Append(t, iterations, forEachIteration{
							Key:   m[keyName.Get(t)],
							Value: m[valueName.Get(t)],
						})
						return nil
					},
				}
			})

			s.Then("the map holds the iteration variables", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var expected []forEachIteration
				for index, element := range elements.Get(t) {
					expected = append(expected, forEachIteration{Key: index, Value: element})
				}
				assert.Equal(t, expected, iterations.Get(t),
					"#ToMap must see the same iteration variables as #Lookup does")
			})
		})

		s.When("the loop body lists its variable bindings", func(s *testcase.Spec) {
			do.Let(s, func(t *testcase.T) workflow.Definition {
				return wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						var vs = workflow.Vars{
							ProcessID:        pid,
							EventsRepository: c.EventRepository.Get(t),
						}
						var bindings = map[workflow.VarName]any{}
						for b, err := range vs.All(ctx) {
							if err != nil {
								return err
							}
							bindings[b.Name] = b.Value
						}
						testcase.Append(t, iterations, forEachIteration{
							Key:   bindings[keyName.Get(t)],
							Value: bindings[valueName.Get(t)],
						})
						return nil
					},
				}
			})

			s.Then("the iteration variables are listed among the bindings", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var expected []forEachIteration
				for index, element := range elements.Get(t) {
					expected = append(expected, forEachIteration{Key: index, Value: element})
				}
				assert.Equal(t, expected, iterations.Get(t),
					"#All must see the same iteration variables as #Lookup does")
			})
		})

		s.When("the collection is a map", func(s *testcase.Spec) {
			var entries = let.Var(s, func(t *testcase.T) map[string]int {
				var m = map[string]int{}
				t.Random.Repeat(2, 5, func() {
					m[t.Random.UUID()] = t.Random.Int()
				})
				return m
			})

			collection.Let(s, func(t *testcase.T) any {
				return entries.Get(t)
			})

			s.Then("the loop body is executed for each entry of the map", func(t *testcase.T) {
				assert.NoError(t, act(t))

				// A map has no iteration order, so the entries the body saw are
				// compared as a set instead of as a sequence.
				var got = map[any]any{}
				for _, iteration := range iterations.Get(t) {
					got[iteration.Key] = iteration.Value
				}

				var expected = map[any]any{}
				for key, value := range entries.Get(t) {
					expected[key] = value
				}

				assert.Equal(t, expected, got,
					"the body must run once per entry,",
					"seeing the map key as the Key and the entry as the Value")
			})
		})

		s.When("the collection is held behind a pointer", func(s *testcase.Spec) {
			// Workflow variables round-trip through reflect on their way in and
			// out of the event log, so a slice or a map that the user holds
			// behind a pointer (*[]T or *map[K]V) must still be recognised as
			// iterable. reflectkit.BaseValue in ForEach#Execute peels the
			// pointer away so the iteration logic sees the underlying slice or
			// map.
			s.When("the slice is held behind a pointer", func(s *testcase.Spec) {
				collection.Let(s, func(t *testcase.T) any {
					vs := elements.Get(t)
					return &vs
				})

				s.Then("the loop body is executed for each element", func(t *testcase.T) {
					assert.NoError(t, act(t))

					var expected []forEachIteration
					for index, element := range elements.Get(t) {
						expected = append(expected, forEachIteration{Key: index, Value: element})
					}
					assert.Equal(t, expected, iterations.Get(t),
						"a *[]T is iterable; the pointer is peeled before the loop sees it")
				})
			})

			s.When("the map is held behind a pointer", func(s *testcase.Spec) {
				var entries = let.Var(s, func(t *testcase.T) map[string]int {
					var m = map[string]int{}
					t.Random.Repeat(2, 5, func() {
						m[t.Random.UUID()] = t.Random.Int()
					})
					return m
				})

				collection.Let(s, func(t *testcase.T) any {
					m := entries.Get(t)
					return &m
				})

				s.Then("the loop body is executed for each entry", func(t *testcase.T) {
					assert.NoError(t, act(t))

					var got = map[any]any{}
					for _, iteration := range iterations.Get(t) {
						got[iteration.Key] = iteration.Value
					}

					var expected = map[any]any{}
					for key, value := range entries.Get(t) {
						expected[key] = value
					}

					assert.Equal(t, expected, got,
						"a *map[K]V is iterable; the pointer is peeled before the loop sees it")
				})
			})
		})

		s.When("the collection is a slice of pointers", func(s *testcase.Spec) {
			// A slice whose elements are pointers is the common shape used to
			// represent "a list of records" without copying each element. The
			// loop must iterate the slice and hand each pointer through to the
			// body, rather than dereferencing and yielding copies.
			type item struct{ Marker string }

			var originals = let.Var(s, func(t *testcase.T) []*item {
				return random.Slice(t.Random.IntBetween(2, 5), func() *item {
					return &item{Marker: t.Random.UUID()}
				})
			})

			collection.Let(s, func(t *testcase.T) any {
				return originals.Get(t)
			})

			s.Then("the body sees each pointer", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var expected []forEachIteration
				for index, ptr := range originals.Get(t) {
					expected = append(expected, forEachIteration{Key: index, Value: ptr})
				}
				assert.Equal(t, expected, iterations.Get(t),
					"each element is a *item; the body must observe the same pointer,",
					"not a copy")
			})
		})

		s.When("the collection is empty", func(s *testcase.Spec) {
			elements.Let(s, func(t *testcase.T) []string {
				return []string{}
			})

			s.Then("the loop body is never executed", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Empty(t, iterations.Get(t))
			})
		})

		s.When("no Key and no Value variable name is configured", func(s *testcase.Spec) {
			keyName.LetValue(s, "")
			valueName.LetValue(s, "")

			s.Then("the loop body is still executed for each element", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, len(elements.Get(t)), len(iterations.Get(t)),
					"Key and Value are optional, leaving them out only means",
					"that the iteration is not exposed as a process variable")
			})
		})

		s.When("the loop body declares a variable", func(s *testcase.Spec) {
			var (
				bodyVarName  = let.Var(s, func(t *testcase.T) workflow.VarName { return workflow.VarName(t.Random.UUID()) })
				bodyVarValue = let.Var[any](s, func(t *testcase.T) any { return t.Random.UUID() })
			)

			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					workflow.DeclareVar{Name: bodyVarName.Get(t)},
					workflow.SetVar{Name: bodyVarName.Get(t), Value: bodyVarValue.Get(t)},
				}
			})

			s.Then("the variable stays inside the loop body and doesn't leak out", func(t *testcase.T) {
				assert.NoError(t, act(t))

				_, ok, err := vars(t).Lookup(ctx.Get(t), bodyVarName.Get(t))
				assert.NoError(t, err)
				assert.False(t, ok,
					"the loop body is a variable scope,",
					"so what is declared within it is not visible from the outside")
			})
		})

		s.When("a variable is already declared outside of the loop", func(s *testcase.Spec) {
			var (
				outerVarName  = let.Var(s, func(t *testcase.T) workflow.VarName { return workflow.VarName(t.Random.UUID()) })
				initialValue  = let.Var[any](s, func(t *testcase.T) any { return t.Random.UUID() })
				assignedValue = let.Var[any](s, func(t *testcase.T) any { return t.Random.UUID() })
			)

			s.Before(func(t *testcase.T) {
				assert.NoError(t, vars(t).Set(ctx.Get(t), outerVarName.Get(t), initialValue.Get(t)))
			})

			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.SetVar{Name: outerVarName.Get(t), Value: assignedValue.Get(t)}
			})

			s.Then("the loop body can assign a new value to it", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok, err := vars(t).Lookup(ctx.Get(t), outerVarName.Get(t))
				assert.NoError(t, err)
				assert.True(t, ok)
				assert.Equal[any](t, assignedValue.Get(t), got,
					"an assignment writes through to the binding the enclosing scope owns")
			})
		})

		s.When("a variable already exists under the name of the loop's Value", func(s *testcase.Spec) {
			var outerValue = let.Var[any](s, func(t *testcase.T) any { return t.Random.UUID() })

			s.Before(func(t *testcase.T) {
				assert.NoError(t, vars(t).Set(ctx.Get(t), valueName.Get(t), outerValue.Get(t)))
			})

			s.Then("the loop body sees the iteration variable instead of the outer one", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var expected []forEachIteration
				for index, element := range elements.Get(t) {
					expected = append(expected, forEachIteration{Key: index, Value: element})
				}
				assert.Equal(t, expected, iterations.Get(t),
					"the iteration variable is declared in the loop body's variable scope,",
					"so within the loop body it shadows the outer variable of the same name")
			})

			s.Then("the iteration variable shadows it instead of overwriting it", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok, err := vars(t).Lookup(ctx.Get(t), valueName.Get(t))
				assert.NoError(t, err)
				assert.True(t, ok)
				assert.Equal[any](t, outerValue.Get(t), got,
					"the iteration variable belongs to the loop,",
					"so iterating must not overwrite an outer variable sharing its name")
			})

			thenARepeatedExecutionSeesTheSame(s)

			s.And("the loop body assigns to the Value variable", func(s *testcase.Spec) {
				var assignedValue = let.Var[any](s, func(t *testcase.T) any { return t.Random.UUID() })

				do.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						workflow.SetVar{Name: valueName.Get(t), Value: assignedValue.Get(t)},
						do.Super(t),
					}
				})

				s.Then("the loop body sees the assigned value", func(t *testcase.T) {
					assert.NoError(t, act(t))

					var expected []forEachIteration
					for index := range elements.Get(t) {
						expected = append(expected, forEachIteration{Key: index, Value: assignedValue.Get(t)})
					}
					assert.Equal(t, expected, iterations.Get(t))
				})

				s.Then("the assignment stays within the loop body, and the outer variable keeps its value", func(t *testcase.T) {
					assert.NoError(t, act(t))

					got, ok, err := vars(t).Lookup(ctx.Get(t), valueName.Get(t))
					assert.NoError(t, err)
					assert.True(t, ok)
					assert.Equal[any](t, outerValue.Get(t), got,
						"within the loop body, the Value name refers to the iteration variable,",
						"so an assignment to it must not write through to the shadowed outer variable")
				})

				thenARepeatedExecutionSeesTheSame(s)

				s.Then("a repeated execution leaves the outer variable as it was too", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.NoError(t, act(t))

					got, ok, err := vars(t).Lookup(ctx.Get(t), valueName.Get(t))
					assert.NoError(t, err)
					assert.True(t, ok)
					assert.Equal[any](t, outerValue.Get(t), got)
				})
			})
		})

		s.When("the loop body deletes the Value variable", func(s *testcase.Spec) {
			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					workflow.DeleteVar{Name: valueName.Get(t)},
					do.Super(t),
				}
			})

			s.Then("the loop body no longer sees it", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var expected []forEachIteration
				for index := range elements.Get(t) {
					expected = append(expected, forEachIteration{Key: index, Value: nil})
				}
				assert.Equal(t, expected, iterations.Get(t),
					"the iteration variable is deleted the same way as a declared one is")
			})

			s.Then("the collection is left as it was", func(t *testcase.T) {
				assert.NoError(t, act(t))

				got, ok, err := vars(t).Lookup(ctx.Get(t), collectionName.Get(t))
				assert.NoError(t, err)
				assert.True(t, ok)
				assert.Equal[any](t, collection.Get(t), got,
					"deleting the iteration variable doesn't delete the element it was taken from")
			})

			thenARepeatedExecutionSeesTheSame(s)
		})

		s.When("the loop body is a nested ForEach", func(s *testcase.Spec) {
			var (
				innerCollectionName = let.Var(s, func(t *testcase.T) workflow.VarName {
					return workflow.VarName(t.Random.UUID())
				})
				innerElements = let.Var(s, func(t *testcase.T) []string {
					return random.Slice(t.Random.IntBetween(1, 3), t.Random.UUID)
				})
			)

			s.Before(func(t *testcase.T) {
				assert.NoError(t, vars(t).Set(ctx.Get(t), innerCollectionName.Get(t), innerElements.Get(t)))
			})

			// The recording loop body moves into the inner loop, where it keeps
			// looking up the Key and Value of the outer loop.
			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.ForEach{
					Over: innerCollectionName.Get(t),
					K:    workflow.VarName(t.Random.UUID()),
					V:    workflow.VarName(t.Random.UUID()),
					Do:   do.Super(t),
				}
			})

			s.Then("the inner loop body still sees the iteration variables of the outer loop", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var expected []forEachIteration
				for index, element := range elements.Get(t) {
					for range innerElements.Get(t) {
						expected = append(expected, forEachIteration{Key: index, Value: element})
					}
				}
				assert.Equal(t, expected, iterations.Get(t),
					"the inner loop body is nested in the outer one's variable scope,",
					"so the outer iteration variables stay visible within it")
			})

			thenARepeatedExecutionSeesTheSame(s)
		})

		s.When("the loop body executes a participant", func(s *testcase.Spec) {
			var (
				participantID = wftest.LetParticipantID(s)
				callCount     = let.VarOf(s, 0)
				_             = wftest.LetParticipantWithID(s, participantID, func(t *testcase.T) func(ctx context.Context) error {
					return func(ctx context.Context) error {
						callCount.Set(t, callCount.Get(t)+1)
						return nil
					}
				})
			)

			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Execute{Participant: participantID.Get(t)}
			})

			s.Then("the participant is called once per element", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, len(elements.Get(t)), callCount.Get(t))
			})

			s.Then("repeating the execution doesn't repeat the iterations", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.NoError(t, act(t))

				assert.Equal(t, len(elements.Get(t)), callCount.Get(t),
					"a workflow Process is replayed on retry,",
					"so an iteration that already ran must not run a second time")
			})

			s.And("a later iteration breaks out of the loop", func(s *testcase.Spec) {
				// breakAt is the iteration at which the body raises the Break.
				// It is picked before the collection runs out, so it is the
				// break that ends the loop, not the collection.
				var breakAt = let.Var(s, func(t *testcase.T) int {
					return t.Random.IntBetween(1, len(elements.Get(t))-1)
				})

				do.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						do.Super(t),
						wftest.Stub{
							StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
								if breakAt.Get(t) <= callCount.Get(t) {
									return workflow.Break{}
								}
								return nil
							},
						},
					}
				})

				s.Then("the participant is called up to the breaking iteration", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, breakAt.Get(t), callCount.Get(t))
				})

				s.Then("repeating the execution doesn't repeat the iterations before the break", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.NoError(t, act(t))

					assert.Equal(t, breakAt.Get(t), callCount.Get(t),
						"the iterations which ran before the break are part of the Process,",
						"so a replay must not run them again")
				})
			})

			s.And("the participant takes the iteration variables as its Input", func(s *testcase.Spec) {
				_ = wftest.LetParticipantWithID(s, participantID, func(t *testcase.T) func(ctx context.Context, index int, element string) error {
					return func(ctx context.Context, index int, element string) error {
						callCount.Set(t, callCount.Get(t)+1)
						return nil
					}
				})

				do.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Execute{
						Participant: participantID.Get(t),
						Input:       []workflow.VarName{keyName.Get(t), valueName.Get(t)},
					}
				})

				s.Then("repeating the execution doesn't repeat the iterations", func(t *testcase.T) {
					assert.NoError(t, act(t))
					assert.NoError(t, act(t))

					assert.Equal(t, len(elements.Get(t)), callCount.Get(t),
						"a recorded call is replayed while its Input is unchanged,",
						"and the iteration variables are part of that Input")
				})
			})
		})

		s.When("the loop body fails", func(s *testcase.Spec) {
			var expErr = let.Error(s)

			do.Let(s, func(t *testcase.T) workflow.Definition {
				return wftest.Stub{
					StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
						testcase.Append(t, iterations, forEachIteration{})
						return expErr.Get(t)
					},
				}
			})

			s.Then("the error is propagated back", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))
			})

			s.Then("the iteration is interrupted", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), expErr.Get(t))

				assert.Equal(t, 1, len(iterations.Get(t)),
					"a failing iteration ends the loop,",
					"the remaining elements are left for the retry to pick up")
			})
		})

		s.When("the loop body breaks out of the loop", func(s *testcase.Spec) {
			// breakAt is the iteration at which the body raises the Break. It is
			// picked before the collection runs out, so it is the break that ends
			// the loop, not the collection.
			var breakAt = let.Var(s, func(t *testcase.T) int {
				return t.Random.IntBetween(1, len(elements.Get(t))-1)
			})

			do.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					do.Super(t),
					wftest.Stub{
						StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
							if breakAt.Get(t) <= len(iterations.Get(t)) {
								return workflow.Break{}
							}
							return nil
						},
					},
				}
			})

			s.Then("the loop ends without an error", func(t *testcase.T) {
				assert.NoError(t, act(t),
					"breaking out of a loop is how a loop ends,",
					"not a failure to report upwards")
			})

			s.Then("the elements after the break are not iterated", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, breakAt.Get(t), len(iterations.Get(t)),
					"the loop ends where the break was raised,",
					"so the rest of the collection is left alone")
			})

			s.And("the collection is a map", func(s *testcase.Spec) {
				// The map is built out of the elements, so the loop has the same
				// number of rounds to break at as it has with a slice.
				collection.Let(s, func(t *testcase.T) any {
					var m = map[string]int{}
					for index, element := range elements.Get(t) {
						m[element] = index
					}
					return m
				})

				s.Then("the entries after the break are not iterated", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Equal(t, breakAt.Get(t), len(iterations.Get(t)),
						"a break ends the loop the same way,",
						"no matter what kind of collection is iterated")
				})
			})
		})

		s.When("the loop has no body", func(s *testcase.Spec) {
			do.Let(s, func(t *testcase.T) workflow.Definition {
				return nil
			})

			s.Then("the iteration completes without an error", func(t *testcase.T) {
				assert.NoError(t, act(t))
			})
		})

		s.When("the collection variable is not present in the process", func(s *testcase.Spec) {
			hasCollection.LetValue(s, false)

			s.Then("the act fails as a fatal workflow error", func(t *testcase.T) {
				err := act(t)
				assert.True(t, workflow.ErrIsFatal(err),
					"a missing collection is a fatal workflow error,",
					"so withRetry will not loop on it")

				assert.Empty(t, iterations.Get(t),
					"a missing collection is a definition mistake,",
					"not an empty iteration")
			})
		})

		s.When("the collection variable holds a value which is not iterable", func(s *testcase.Spec) {
			collection.Let(s, func(t *testcase.T) any {
				return t.Random.Int()
			})

			s.Then("nothing is iterated", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Empty(t, iterations.Get(t))
			})
		})

		s.When("the collection variable holds a nil pointer", func(s *testcase.Spec) {
			// A workflow variable may be any any-shaped value, and a nil pointer
			// to a slice or a map is the natural "no collection" shape that
			// reflectkit.BaseValue stops at. The loop must treat it like an
			// empty collection: nothing is iterated, no error is returned.
			s.And("to a slice", func(s *testcase.Spec) {
				collection.Let(s, func(t *testcase.T) any {
					var vs *[]string
					return vs
				})

				s.Then("nothing is iterated", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Empty(t, iterations.Get(t))
				})
			})

			s.And("to a map", func(s *testcase.Spec) {
				collection.Let(s, func(t *testcase.T) any {
					var m *map[string]int
					return m
				})

				s.Then("nothing is iterated", func(t *testcase.T) {
					assert.NoError(t, act(t))

					assert.Empty(t, iterations.Get(t))
				})
			})
		})
	})

	s.Context("var-scope", func(s *testcase.Spec) {
		var seen = let.VarOf[[]any](s, nil)

		s.Test("SetVar on #V within #Do will update the variables inner scope", func(t *testcase.T) {
			def := workflow.Sequence{
				workflow.DeclareVar{Name: "vs"},
				workflow.SetVar{Name: "vs", Value: []string{"foo", "bar", "baz"}},
				workflow.ForEach{
					Over: "vs", V: "e",
					Do: workflow.Sequence{
						workflow.SetVar{Name: "e", Value: "qux"},
						wftest.Stub{
							StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
								vars, err := workflow.GetVars(ctx)
								assert.NoError(t, err)

								v, ok, err := vars.Lookup(ctx, "e")
								assert.NoError(t, err)
								assert.True(t, ok)
								assert.Equal(t, v, "qux")
								vs, err := vars.Get(ctx, "vs")
								assert.NoError(t, err)
								assert.Equal[any](t, []string{"foo", "bar", "baz"}, vs)
								return nil
							},
						},
					},
				},
			}

			pid := wftest.MakeProcessID(t)
			assert.NoError(t, wftest.Runtime.Get(t).Bind(t.Context(), pid, def))
			assert.NoError(t, wftest.Runtime.Get(t).Execute(t.Context(), pid))
		})

		s.Test("golang mid-flight collection mutation reference", func(t *testcase.T) {
			vs := []string{"foo", "bar", "baz"}
			for i, v := range vs {
				if i == 0 {
					vs[2] = "qux"
				}
				if i == 2 {
					assert.Equal(t, v, "qux")
				}
			}
		})

		s.Test("update on the collection itself will update the iteration value too", func(t *testcase.T) {

			def := workflow.Sequence{
				workflow.DeclareVar{Name: "vs"},
				workflow.SetVar{Name: "vs", Value: []string{"foo", "bar", "baz"}},
				workflow.ForEach{
					Over: "vs", V: "e",
					Do: workflow.Sequence{
						workflow.SetVar{Name: "vs", Value: []string{"qux", "qux", "qux"}},
						wftest.Stub{
							StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
								vars, err := workflow.GetVars(ctx)
								assert.NoError(t, err)

								v, ok, err := vars.Lookup(ctx, "e")
								assert.NoError(t, err)
								assert.True(t, ok)
								assert.Equal(t, v, "qux",
									"the body reassigned the collection to put 'qux' at every index,",
									"so the iteration value must reflect that on every iteration,",
									"not the cached element from iteration-start")
								return nil
							},
						},
					},
				},
			}

			pid := wftest.MakeProcessID(t)
			assert.NoError(t, wftest.Runtime.Get(t).Bind(t.Context(), pid, def))
			assert.NoError(t, wftest.Runtime.Get(t).Execute(t.Context(), pid))
		})

		s.Test("a nested loop body sees the iteration variables of both loops", func(t *testcase.T) {
			// rows := []string{"r1", "r2"}
			// cols := []string{"c1"}
			// for _, row := range rows {
			// 	for _, col := range cols {
			// 		seen = append(seen, row+":"+col)
			// 	}
			// }
			var seen []string
			def := workflow.Sequence{
				workflow.DeclareVar{Name: "rows"},
				workflow.SetVar{Name: "rows", Value: []string{"r1", "r2"}},
				workflow.DeclareVar{Name: "cols"},
				workflow.SetVar{Name: "cols", Value: []string{"c1"}},
				workflow.ForEach{
					Over: "rows", V: "row",
					Do: workflow.ForEach{
						Over: "cols", V: "col",
						Do: wftest.Stub{
							StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
								vars, err := workflow.GetVars(ctx)
								if err != nil {
									return err
								}
								row, _, err := vars.Lookup(ctx, "row")
								if err != nil {
									return err
								}
								col, _, err := vars.Lookup(ctx, "col")
								if err != nil {
									return err
								}
								seen = append(seen, fmt.Sprintf("%v:%v", row, col))
								return nil
							},
						},
					},
				},
			}

			pid := wftest.MakeProcessID(t)
			assert.NoError(t, wftest.Runtime.Get(t).Bind(t.Context(), pid, def))
			assert.NoError(t, wftest.Runtime.Get(t).Execute(t.Context(), pid))

			assert.Equal(t, []string{"r1:c1", "r2:c1"}, seen,
				"the inner loop must not hide the outer loop's `row`")
		})

		s.Test("the iteration variable shadows an outer variable of the same name", func(t *testcase.T) {
			// item := "outer"
			// items := []string{"a", "b"}
			// for _, item := range items {
			// 	seen = append(seen, item)
			// }
			// // item == "outer"
			var seen []any
			def := workflow.Sequence{
				workflow.DeclareVar{Name: "item"},
				workflow.SetVar{Name: "item", Value: "outer"},
				workflow.DeclareVar{Name: "items"},
				workflow.SetVar{Name: "items", Value: []string{"a", "b"}},
				workflow.ForEach{
					Over: "items", V: "item",
					Do: wftest.Stub{
						StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
							vars, err := workflow.GetVars(ctx)
							if err != nil {
								return err
							}
							item, _, err := vars.Lookup(ctx, "item")
							if err != nil {
								return err
							}
							seen = append(seen, item)
							return nil
						},
					},
				},
			}

			pid := wftest.MakeProcessID(t)
			assert.NoError(t, wftest.Runtime.Get(t).Bind(t.Context(), pid, def))
			assert.NoError(t, wftest.Runtime.Get(t).Execute(t.Context(), pid))

			assert.Equal(t, []any{"a", "b"}, seen,
				"within the loop body, `item` is the iteration variable")

			vars := workflow.Vars{ProcessID: pid, EventsRepository: wftest.EventRepository.Get(t)}
			item, err := vars.Get(t.Context(), "item")
			assert.NoError(t, err)
			assert.Equal[any](t, "outer", item,
				"after the loop, `item` is the outer variable again")
		})

		s.Test("an assignment to a shadowing iteration variable stays within the loop body", func(t *testcase.T) {
			// item := "outer"
			// items := []string{"a", "b"}
			// for _, item := range items {
			// 	item = "changed-in-loop"
			// 	seen = append(seen, item)
			// }
			// // item == "outer"
			var seen []any
			def := workflow.Sequence{
				workflow.DeclareVar{Name: "item"},
				workflow.SetVar{Name: "item", Value: "outer"},
				workflow.DeclareVar{Name: "items"},
				workflow.SetVar{Name: "items", Value: []string{"a", "b"}},
				workflow.ForEach{
					Over: "items", V: "item",
					Do: workflow.Sequence{
						workflow.SetVar{Name: "item", Value: "changed-in-loop"},
						wftest.Stub{
							StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
								vars, err := workflow.GetVars(ctx)
								if err != nil {
									return err
								}
								item, _, err := vars.Lookup(ctx, "item")
								if err != nil {
									return err
								}
								seen = append(seen, item)
								return nil
							},
						},
					},
				},
			}

			pid := wftest.MakeProcessID(t)
			assert.NoError(t, wftest.Runtime.Get(t).Bind(t.Context(), pid, def))
			assert.NoError(t, wftest.Runtime.Get(t).Execute(t.Context(), pid))

			assert.Equal(t, []any{"changed-in-loop", "changed-in-loop"}, seen,
				"within the loop body, the assignment is visible")

			vars := workflow.Vars{ProcessID: pid, EventsRepository: wftest.EventRepository.Get(t)}
			item, err := vars.Get(t.Context(), "item")
			assert.NoError(t, err)
			assert.Equal[any](t, "outer", item,
				"the assignment targeted the iteration variable,",
				"so it must not write through to the shadowed outer `item`")
		})

		s.Test("a sibling loop doesn't see the variables of the loop before it", func(t *testcase.T) {
			// items := []string{"a", "b"}
			// for range items {
			// 	x := "declared-in-first-loop"
			// }
			// for range items {
			// 	see(x)
			// }
			_, err := execProcess(t, c.Runtime.Get(t), workflow.Sequence{
				workflow.SetVar{Name: "items", Value: []string{"a", "b"}},
				workflow.ForEach{
					Over: "items",
					Do:   workflow.SetVar{Name: "x", Value: "declared-in-first-loop"},
				},
				workflow.ForEach{
					Over: "items",
					Do:   peekVar(t, seen, "x"),
				},
			})
			assert.NoError(t, err)

			assert.Equal(t, []any{undeclared, undeclared}, seen.Get(t),
				"the iterations of two loops are different blocks, even at the same position of their collections")
		})

		s.Test("a sibling loop has iteration variables of its own, whatever the loop before it assigned to its own", func(t *testcase.T) {
			// items := []string{"a", "b"}
			// for _, item := range items {
			// 	item = "changed-in-first-loop"
			// }
			// for _, item := range items {
			// 	see(item)
			// }
			_, err := execProcess(t, c.Runtime.Get(t), workflow.Sequence{
				workflow.SetVar{Name: "items", Value: []string{"a", "b"}},
				workflow.ForEach{
					Over: "items", V: "item",
					Do: workflow.SetVar{Name: "item", Value: "changed-in-first-loop"},
				},
				workflow.ForEach{
					Over: "items", V: "item",
					Do: peekVar(t, seen, "item"),
				},
			})
			assert.NoError(t, err)

			assert.Equal(t, []any{"a", "b"}, seen.Get(t))
		})

		s.Test("a loop nested in the body of a For starts over with fresh variables on every round", func(t *testcase.T) {
			// items := []string{"a"}
			// for i := 0; i < 2; i++ {
			// 	for range items {
			// 		see(x) // undeclared on every round
			// 		x := "declared-in-nested-loop"
			// 	}
			// }
			_, err := execProcess(t, c.Runtime.Get(t), workflow.Sequence{
				workflow.SetVar{Name: "items", Value: []string{"a"}},
				workflow.For{
					Init: workflow.SetVar{Name: "i", Value: 0},
					Cond: wftemplate.Condition("lt .i 2"),
					Post: workflow.Increment{Name: "i"},
					Do: workflow.ForEach{
						Over: "items",
						Do: workflow.Sequence{
							peekVar(t, seen, "x"),
							workflow.SetVar{Name: "x", Value: "declared-in-nested-loop"},
						},
					},
				},
			})
			assert.NoError(t, err)

			assert.Equal(t, []any{undeclared, undeclared}, seen.Get(t))
		})
	})

	// A Process which suspends is executed again later, and that execution
	// replays every step the earlier ones already did. A replayed participant
	// call is only skipped while the variables it takes as its Input resolve to
	// the same values they did when the call was recorded. So if a variable
	// drifted between the executions, the participant would be called again,
	// and it would show up in `seen` a second time.
	s.Context("a resumed Process replays the loop with the same variables", func(s *testcase.Spec) {
		var (
			seen      = let.VarOf[[]string](s, nil)
			suspended = let.VarOf(s, false)
		)
		wftest.Participants.Let(s, func(t *testcase.T) workflow.Participants {
			return workflow.Participants{
				"see": func(ctx context.Context, v string) error {
					testcase.Append(t, seen, v)
					return nil
				},
				"see-pair": func(ctx context.Context, a, b string) error {
					testcase.Append(t, seen, a+":"+b)
					return nil
				},
				"suspend-once": func(ctx context.Context) error {
					if suspended.Get(t) {
						return nil
					}
					suspended.Set(t, true)
					return workflow.Suspend{}
				},
			}
		})

		// run executes the definition as a Process, which suspends once, and then
		// resumes it, replaying everything the first execution did.
		var run = func(t *testcase.T, def workflow.Definition) workflow.ProcessID {
			pid := wftest.MakeProcessID(t)
			assert.Must(t).NoError(wftest.Runtime.Get(t).Bind(t.Context(), pid, def))
			assert.Must(t).ErrorIs(wftest.Runtime.Get(t).Execute(t.Context(), pid), workflow.Suspend{})
			assert.Must(t).NoError(wftest.Runtime.Get(t).Execute(t.Context(), pid))
			return pid
		}

		s.Test("the iteration variables of nested loops", func(t *testcase.T) {
			// rows := []string{"r1", "r2"}
			// cols := []string{"c1", "c2"}
			// for _, row := range rows {
			// 	for _, col := range cols {
			// 		see(row, col)
			// 	}
			// }
			// suspend()
			run(t, workflow.Sequence{
				workflow.SetVar{Name: "rows", Value: []string{"r1", "r2"}},
				workflow.SetVar{Name: "cols", Value: []string{"c1", "c2"}},
				workflow.ForEach{
					Over: "rows", V: "row",
					Do: workflow.ForEach{
						Over: "cols", V: "col",
						Do: workflow.Execute{Participant: "see-pair", Input: []workflow.VarName{"row", "col"}},
					},
				},
				workflow.Execute{Participant: "suspend-once"},
			})

			assert.Equal(t, []string{"r1:c1", "r1:c2", "r2:c1", "r2:c2"}, seen.Get(t),
				"every pair is seen exactly once, and the resumed execution replays each of them")
		})

		s.Test("a shadowing iteration variable, and the outer variable it shadows", func(t *testcase.T) {
			// item := "outer"
			// items := []string{"a", "b"}
			// for _, item := range items {
			// 	see(item)
			// 	item = "changed-in-loop"
			// 	see(item)
			// }
			// see(item)
			// item = "after"
			// suspend()
			pid := run(t, workflow.Sequence{
				workflow.SetVar{Name: "item", Value: "outer"},
				workflow.SetVar{Name: "items", Value: []string{"a", "b"}},
				workflow.ForEach{
					Over: "items", V: "item",
					Do: workflow.Sequence{
						workflow.Execute{Participant: "see", Input: []workflow.VarName{"item"}},
						workflow.SetVar{Name: "item", Value: "changed-in-loop"},
						workflow.Execute{Participant: "see", Input: []workflow.VarName{"item"}},
					},
				},
				workflow.Execute{Participant: "see", Input: []workflow.VarName{"item"}},
				workflow.SetVar{Name: "item", Value: "after"},
				workflow.Execute{Participant: "suspend-once"},
			})

			assert.Equal(t, []string{"a", "changed-in-loop", "b", "changed-in-loop", "outer"}, seen.Get(t),
				"every value is seen exactly once, and the resumed execution replays each of them")

			vars := workflow.Vars{ProcessID: pid, EventsRepository: wftest.EventRepository.Get(t)}
			item, err := vars.Get(t.Context(), "item")
			assert.NoError(t, err)
			assert.Equal[any](t, "after", item,
				"the assignment after the loop is made to the outer `item`")
		})

		s.Test("the iteration variables on every round of an enclosing For", func(t *testcase.T) {
			// items := []string{"a", "b"}
			// for i := 0; i < 2; i++ {
			// 	for _, item := range items {
			// 		see(item)
			// 		item = "changed-in-loop"
			// 	}
			// }
			// suspend()
			run(t, workflow.Sequence{
				workflow.SetVar{Name: "items", Value: []string{"a", "b"}},
				workflow.For{
					Init: workflow.SetVar{Name: "i", Value: 0},
					Cond: wftemplate.Condition("lt .i 2"),
					Post: workflow.Increment{Name: "i"},
					Do: workflow.ForEach{
						Over: "items", V: "item",
						Do: workflow.Sequence{
							workflow.Execute{Participant: "see", Input: []workflow.VarName{"item"}},
							workflow.SetVar{Name: "item", Value: "changed-in-loop"},
						},
					},
				},
				workflow.Execute{Participant: "suspend-once"},
			})

			assert.Equal(t, []string{"a", "b", "a", "b"}, seen.Get(t),
				"the loop body runs in the same variable scope on every round of the For,",
				"but every round starts the loop over with fresh iteration variables,",
				"not with what an earlier round assigned to them")
		})
	})
}
