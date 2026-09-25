package workflow_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/let"
)

func TestIf_replay(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		process     = wftest.LetProcessID(s)
		cid         = LetConditionID(s)
		answer      = let.VarOf(s, true)
		failure     = let.VarOf[error](s, nil)
		evaluations = let.VarOf(s, 0)
		calls       = let.Var(s, func(t *testcase.T) []string { return nil })
		paths       = let.Var(s, func(t *testcase.T) []workflow.Path { return nil })
		evaluate    = let.Var(s, func(t *testcase.T) func(context.Context, workflow.ProcessID) (bool, error) {
			return func(context.Context, workflow.ProcessID) (bool, error) {
				evaluations.Set(t, evaluations.Get(t)+1)
				return answer.Get(t), failure.Get(t)
			}
		})
		// condition is a custom Condition which makes its answers replay-stable
		// the recommended way: by answering through Execute#EvaluateWith.
		condition = let.Var[workflow.Condition](s, func(t *testcase.T) workflow.Condition {
			return wftest.Stub{StubEvaluate: func(ctx context.Context, pid workflow.ProcessID) (bool, error) {
				return workflow.Execute{}.EvaluateWith(ctx, pid, cid.Get(t), evaluate.Get(t))
			}}
		})
		branchFailure = let.VarOf[error](s, nil)
		ctx           = let.Var(s, func(t *testcase.T) context.Context { return t.Context() })
	)
	wftest.Participants.Let(s, func(t *testcase.T) workflow.Participants {
		return workflow.Participants{
			"then": func(ctx context.Context) error {
				testcase.Append(t, calls, "then")
				testcase.Append(t, paths, workflow.CurrentPath(ctx))
				return branchFailure.Get(t)
			},
			"else": func(ctx context.Context) error {
				testcase.Append(t, calls, "else")
				testcase.Append(t, paths, workflow.CurrentPath(ctx))
				return branchFailure.Get(t)
			},
		}
	})
	subject := let.Var(s, func(t *testcase.T) workflow.If {
		return workflow.If{
			Cond: condition.Get(t),
			Then: workflow.Execute{ParticipantID: "then"},
			Else: workflow.Execute{ParticipantID: "else"},
		}
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Execute(wftest.Runtime.Get(t).Context(ctx.Get(t)), process.Get(t))
		}

		s.Then("replays the condition's recorded answer rather than asking it again", func(t *testcase.T) {
			assert.NoError(t, act(t))
			answer.Set(t, false)
			assert.NoError(t, act(t))
			assert.Equal(t, calls.Get(t), []string{"then"})
			assert.Equal(t, evaluations.Get(t), 1)
			assert.Equal(t, paths.Get(t), []workflow.Path{{"if", "then", "participant", "then"}})
		})

		s.Then("the answer is recorded by the condition itself, at the if/<condition ID> path", func(t *testcase.T) {
			assert.NoError(t, act(t))
			events := conditionEvents(t, wftest.EventRepository.Get(t), process.Get(t))
			assert.Must(t).Equal(len(events), 1)
			assert.Equal(t, events[0].ConditionID, cid.Get(t))
			assert.Equal(t, events[0].Path, workflow.Path{"if", string(cid.Get(t))})
			assert.Equal(t, events[0].Answer, answer.Get(t))
		})

		for _, initial := range []bool{true, false} {
			s.When("the selected branch immediately suspends with answer "+map[bool]string{true: "true", false: "false"}[initial], func(s *testcase.Spec) {
				answer.LetValue(s, initial)
				branchFailure.LetValue(s, workflow.Suspend{})

				s.Then("commits the decision before the branch completes", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), workflow.Suspend{})
					answer.Set(t, !initial)
					branchFailure.Set(t, nil)
					assert.NoError(t, act(t))
					branch := map[bool]string{true: "then", false: "else"}[initial]
					assert.Equal(t, calls.Get(t), []string{branch, branch})
					assert.Equal(t, evaluations.Get(t), 1)
					assert.Equal(t, paths.Get(t), []workflow.Path{{"if", branch, "participant", branch}, {"if", branch, "participant", branch}})
				})
			})
		}

		s.When("condition evaluation fails", func(s *testcase.Spec) {
			failure.Let(s, func(t *testcase.T) error { return t.Random.Error() })

			s.Then("does not cache the failed answer and retries evaluation", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), failure.Get(t))
				assert.Empty(t, calls.Get(t))
				failure.Set(t, nil)
				answer.Set(t, false)
				assert.NoError(t, act(t))
				answer.Set(t, true)
				assert.NoError(t, act(t))
				assert.Equal(t, calls.Get(t), []string{"else"})
				assert.Equal(t, evaluations.Get(t), 2)
			})
		})

		s.When("the condition reads a scoped variable", func(s *testcase.Spec) {
			condition.LetValue(s, wftemplate.Condition(".flag"))

			s.Then("different named scope paths keep independent decisions", func(t *testcase.T) {
				vars := workflow.Vars{ProcessID: process.Get(t), EventsRepository: wftest.EventRepository.Get(t)}
				left := workflow.WithVarScope(workflow.WithName(t.Context(), "left"), "left")
				right := workflow.WithVarScope(workflow.WithName(t.Context(), "right"), "right")
				assert.NoError(t, vars.Set(left, "flag", true))
				assert.NoError(t, vars.Set(right, "flag", false))
				ctx.Set(t, left)
				assert.NoError(t, act(t))
				ctx.Set(t, right)
				assert.NoError(t, act(t))
				assert.Equal(t, calls.Get(t), []string{"then", "else"})
			})
		})

		s.When("the condition does not record its answers", func(s *testcase.Spec) {
			condition.Let(s, func(t *testcase.T) workflow.Condition {
				return wftest.Stub{StubEvaluate: evaluate.Get(t)}
			})

			s.Then("If does not record it either, so a replay asks again and follows the new answer", func(t *testcase.T) {
				assert.NoError(t, act(t))
				answer.Set(t, false)
				assert.NoError(t, act(t))
				assert.Equal(t, evaluations.Get(t), 2)
				assert.Equal(t, calls.Get(t), []string{"then", "else"})
				assert.Empty(t, conditionEvents(t, wftest.EventRepository.Get(t), process.Get(t)))
			})
		})

		for _, form := range []string{"value", "pointer"} {
			s.When("the condition is an Execute with a registered ConditionID as a "+form, func(s *testcase.Spec) {
				condition.Let(s, func(t *testcase.T) workflow.Condition {
					registered := workflow.Execute{ConditionID: cid.Get(t), Input: []workflow.VarName{"flag"}}
					if form == "pointer" {
						return &registered
					}
					return registered
				})
				wftest.Conditions.Let(s, func(t *testcase.T) workflow.Conditions {
					return workflow.Conditions{cid.Get(t): func(context.Context, bool) (bool, error) {
						evaluations.Set(t, evaluations.Get(t)+1)
						return answer.Get(t), nil
					}}
				})
				s.Before(func(t *testcase.T) {
					vars := workflow.Vars{ProcessID: process.Get(t), EventsRepository: wftest.EventRepository.Get(t)}
					assert.Must(t).NoError(vars.Set(t.Context(), "flag", true))
				})

				s.Then("a single answer is recorded with its input at the if/<condition ID> path and replayed", func(t *testcase.T) {
					assert.NoError(t, act(t))
					answer.Set(t, false)
					assert.NoError(t, act(t))
					assert.Equal(t, calls.Get(t), []string{"then"})
					assert.Equal(t, evaluations.Get(t), 1)
					events := conditionEvents(t, wftest.EventRepository.Get(t), process.Get(t))
					assert.Must(t).Equal(len(events), 1)
					assert.Equal(t, events[0].Path, workflow.Path{"if", string(cid.Get(t))})
					assert.Equal(t, events[0].Input, []any{true})
				})

				s.Then("the answer is part of the caller's transaction and discarded with its rollback", func(t *testcase.T) {
					repo := wftest.EventRepository.Get(t)
					before := getProcessEvents(t, repo, process.Get(t))
					tx, err := repo.BeginTx(ctx.Get(t))
					assert.Must(t).NoError(err)
					t.Cleanup(func() { _ = repo.RollbackTx(tx) })
					ctx.Set(t, tx)
					assert.NoError(t, act(t))
					assert.Equal(t, getProcessEvents(t, repo, process.Get(t)), before)
					assert.NoError(t, repo.RollbackTx(tx))
					ctx.Set(t, t.Context())
					assert.NoError(t, act(t))
					assert.Equal(t, evaluations.Get(t), 2)
					assert.Equal(t, calls.Get(t), []string{"then", "then"})
				})

				s.And("its answer was recorded before the registered condition changed", func(s *testcase.Spec) {
					answer.LetValue(s, false)
					s.Before(func(t *testcase.T) {
						repo := wftest.EventRepository.Get(t)
						var event workflow.Event = workflow.EventCondition{
							EventID: mustEventID(t), ProcessID: process.Get(t), Timestamp: clock.Now(),
							ConditionID: cid.Get(t),
							Path:        workflow.Path{"if", string(cid.Get(t))},
							Input:       []any{true}, Answer: true,
						}
						assert.Must(t).NoError(repo.Create(t.Context(), &event))
						vars := workflow.Vars{ProcessID: process.Get(t), EventsRepository: repo}
						assert.Must(t).NoError(vars.Set(t.Context(), "flag", false))
					})

					s.Then("the existing history replays its answer without asking the registry", func(t *testcase.T) {
						assert.NoError(t, act(t))
						assert.NoError(t, act(t))
						assert.Equal(t, evaluations.Get(t), 0)
						assert.Equal(t, calls.Get(t), []string{"then"})
						assert.Equal(t, len(conditionEvents(t, wftest.EventRepository.Get(t), process.Get(t))), 1)
					})
				})
			})
		}
	})

	s.Describe("template branch replay in a sequence", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return wftest.Runtime.Get(t).Execute(t.Context(), process.Get(t))
		}
		condition.LetValue(s, wftemplate.Condition(".flag"))
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
		s.Before(func(t *testcase.T) {
			branch := subject.Get(t)
			branch.Then = workflow.Sequence{branch.Then, workflow.SetVar{Name: "flag", Value: false}}
			definition := workflow.Sequence{
				workflow.SetVar{Name: "flag", Value: true},
				branch,
				workflow.Execute{ParticipantID: "suspend-once"},
			}
			assert.Must(t).NoError(wftest.Runtime.Get(t).Bind(t.Context(), process.Get(t), definition))
		})

		s.Then("does not execute Else after Then changes the condition input and execution resumes", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), workflow.Suspend{})
			assert.NoError(t, act(t))
			assert.Equal(t, calls.Get(t), []string{"then"})
			assert.Equal(t, suspendCalls.Get(t), 2)
			vars := workflow.Vars{ProcessID: process.Get(t), EventsRepository: wftest.EventRepository.Get(t)}
			value, err := vars.Get(t.Context(), "flag")
			assert.NoError(t, err)
			assert.Equal[any](t, value, false)
		})
	})

	s.Describe("loop iteration decisions", func(s *testcase.Spec) {
		loop := let.Var(s, func(t *testcase.T) workflow.For {
			return workflow.For{
				Init: workflow.SetVar{Name: "i", Value: 0},
				Cond: workflow.Execute{ConditionID: "continue", Input: []workflow.VarName{"i"}},
				Post: workflow.Increment{Name: "i"},
				Do:   subject.Get(t),
			}
		})
		act := func(t *testcase.T) error {
			return loop.Get(t).Execute(wftest.Runtime.Get(t).Context(t.Context()), process.Get(t))
		}
		condition.LetValue(s, wftemplate.Condition("eq .i 0"))
		wftest.Conditions.Let(s, func(t *testcase.T) workflow.Conditions {
			return workflow.Conditions{"continue": func(ctx context.Context, i int) (bool, error) { return i < 2, nil }}
		})

		s.Then("each round chooses its own branch and retains that choice on replay", func(t *testcase.T) {
			assert.NoError(t, act(t))
			assert.NoError(t, act(t))
			assert.Equal(t, calls.Get(t), []string{"then", "else"})
		})
	})
}
