package workflow_test

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftest"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func ExampleSpawn() {
	_ = workflow.Spawn{
		Definition: workflow.Sequence{},
		Vars: workflow.VarMapping{
			"parent-key": "child-key",
		},
	}
}

var _ workflow.Definition = workflow.Spawn{}

func TestSpawn(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		expVal1 = let.String(s)
		expVal2 = let.Int(s)

		blockingParticipantID     = wftest.LetParticipantID(s)
		blockingParticipantPhaser = let.Phaser(s)
		_                         = wftest.LetParticipantWithID(s, c, blockingParticipantID, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				blockingParticipantPhaser.Get(t).Wait()
				return nil
			}
		})
	)
	subject := let.Var(s, func(t *testcase.T) workflow.Spawn {
		return workflow.Spawn{
			Name: workflow.SpawnName(t.Random.UUID()),
			Definition: workflow.Sequence{
				workflow.ExecuteParticipant{ID: blockingParticipantID.Get(t)},
				workflow.SetVar{Name: "sub-wf-key-in-def", Value: expVal1.Get(t)},
				workflow.SetVar{Name: "sub-wf-key-in-spawn", Value: expVal2.Get(t)},
			},
		}
	})

	_, spawnerParticipantID := wftest.LetParticipant(s, c, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			return subject.Get(t)
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		if t.Random.Bool() {
			t.Log("spawn can be used as an participant signal result value too")
			return workflow.ExecuteParticipant{ID: spawnerParticipantID.Get(t)}
		}
		return subject.Get(t)
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) error {
			return c.ActExecute(t)
		})

		s.Then("workflow execution will NOT wait on the subprocess", func(t *testcase.T) {
			assert.Within(t, deadline, func(ctx context.Context) {
				assert.NoError(t, act(t))
			})

			c.WaitForSpawn(t, c.ProcessID.Get(t))
			c.ProcessCompletionIs(t, c.ProcessID.Get(t), true)
			c.ChildrenCompletionAre(t, c.ProcessID.Get(t), false)
			blockingParticipantPhaser.Get(t).Finish()
			c.ChildrenCompletionAre(t, c.ProcessID.Get(t), true)
		})

		s.Then("spawned sub-process completion is independent from the parent process", func(t *testcase.T) {
			assert.Within(t, deadline, func(ctx context.Context) {
				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))
				})
			})

			c.WaitForSpawn(t, c.ProcessID.Get(t))
			c.ProcessCompletionIs(t, c.ProcessID.Get(t), true)

			t.Log("due to being blocked, the child is not yet completed unlike its parent")
			c.ChildrenCompletionAre(t, c.ProcessID.Get(t), false)

			blockingParticipantPhaser.Get(t).Finish()
			t.Log("but after no more blocking issues are present for the child, it finished up too")
			c.ChildrenCompletionAre(t, c.ProcessID.Get(t), true)
		})

		s.Then("multiple execution won't spawn multiple sub process", func(t *testcase.T) {
			blockingParticipantPhaser.Get(t).Finish()

			assert.Within(t, deadline, func(ctx context.Context) {
				t.Random.Repeat(3, 7, func() {
					assert.NoError(t, act(t))
				})
			})

			c.WaitForSpawn(t, c.ProcessID.Get(t))
			time.Sleep(waitTime)
			t.Random.Repeat(3, 7, func() {
				runtime.Gosched()
				events, err := iterkit.CollectE(c.EventRepository.Get(t).FindByProcessID(t.Context(), c.ProcessID.Get(t)))
				assert.NoError(t, err)

				events = slicekit.Filter(events, func(e workflow.Event) bool {
					_, ok := e.(workflow.EventSpawn)
					return ok
				})

				assert.Equal(t, len(events), 1,
					"it was expected that the spawning is also idempotent, and not repeated")
			})

			c.ProcessCompletionIs(t, c.ProcessID.Get(t), true)
			c.ChildrenCompletionAre(t, c.ProcessID.Get(t), true)
		})
	})

}

// TestSpawn_Vars asserts the parent->child variable forwarding
// behaviour introduced when Spawn#Vars was retyped from a literal map to a
// workflow.VarMapping.
//
// Each VarMapping entry {parentKey: childKey} instructs the spawn to look up
// the current value of parentKey on the parent process and copy it under
// childKey on the spawned child. Parent keys that have no value at spawn time
// are skipped silently rather than aborting the spawn.
func TestSpawn_Vars(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	var (
		// parentValue is the value we put on the parent and expect to see on
		// the child under the mapped child key.
		parentValue = let.String(s)
		// childKey is the key the parent value lands under on the child.
		childKey = let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.UUID())
		})
		// parentKey is the key the parent value sits under on the parent.
		parentKey = let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.UUID())
		})
		// missingKey is a key set on the parent but expected to NOT be
		// forwarded to the child (the test for the silent-skip contract).
		missingParentKey = let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.UUID())
		})
		missingChildKey = let.Var(s, func(t *testcase.T) workflow.VarName {
			return workflow.VarName(t.Random.UUID())
		})
	)

	childID := let.Var(s, func(t *testcase.T) workflow.ProcessID {
		events := c.ProcessEvents(t, c.ProcessID.Get(t))
		for _, e := range events {
			if spawn, ok := e.(workflow.EventSpawn); ok {
				return spawn.ChildID
			}
		}
		t.Fatalf("expected a SpawnEvent to be recorded for the parent process")
		return workflow.ProcessID{}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.SetVar{Name: parentKey.Get(t), Value: parentValue.Get(t)},
			workflow.Spawn{
				Name: workflow.SpawnName(t.Random.UUID()),
				Definition: workflow.Sequence{
					workflow.SetVar{Name: "sentinel", Value: "child-finished"},
				},
				Vars: workflow.VarMapping{
					parentKey.Get(t):        childKey.Get(t),
					missingParentKey.Get(t): missingChildKey.Get(t),
				},
			},
		}
	})

	s.Then("parent variables are forwarded to the spawned child under the mapped child key", func(t *testcase.T) {
		assert.NoError(t, c.ActExecute(t))

		c.WaitForSpawn(t, c.ProcessID.Get(t))

		// Eventually: the child history must show the parent value under
		// the child key.
		t.Eventually(func(t *testcase.T) {
			childEvents := c.ProcessEvents(t, childID.Get(t))
			for _, e := range childEvents {
				ve, ok := e.(workflow.EventSetVar)
				if !ok {
					continue
				}
				if ve.Name == childKey.Get(t) {
					assert.Equal(t, parentValue.Get(t), ve.Value.(string))
					return
				}
			}
			t.Fatalf("expected child to have variable %q set from parent", childKey.Get(t))
		})
	})

	s.Then("a parent key with no value is silently skipped, not an error", func(t *testcase.T) {
		assert.NoError(t, c.ActExecute(t))

		c.WaitForSpawn(t, c.ProcessID.Get(t))
		c.ProcessCompletionIs(t, c.ProcessID.Get(t), true)

		childEvents := c.ProcessEvents(t, childID.Get(t))
		for _, e := range childEvents {
			ve, ok := e.(workflow.EventSetVar)
			if !ok {
				continue
			}
			assert.NotEqual(t, ve.Name, missingChildKey.Get(t),
				"expected that a missing parent value is not forwarded")
		}
	})

	s.When("the spawned child reassigns the forwarded variable", func(s *testcase.Spec) {
		// The child's own definition overwrites what it received from the
		// parent, so by the time the child is done, the forwarded binding no
		// longer reflects the parent's value.
		childValue := let.Var(s, func(t *testcase.T) string {
			return random.Unique(t.Random.String, parentValue.Get(t))
		})

		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.SetVar{Name: parentKey.Get(t), Value: parentValue.Get(t)},
				workflow.Spawn{
					Name: workflow.SpawnName(t.Random.UUID()),
					Definition: workflow.Sequence{
						workflow.SetVar{Name: childKey.Get(t), Value: childValue.Get(t)},
					},
					Vars: workflow.VarMapping{
						parentKey.Get(t): childKey.Get(t),
					},
				},
			}
		})

		s.Then("replaying the parent does not forward the parent value over the child's own", func(t *testcase.T) {
			assert.NoError(t, c.ActExecute(t))

			c.WaitForSpawn(t, c.ProcessID.Get(t))
			c.ChildrenCompletionAre(t, c.ProcessID.Get(t), true)

			assert.Equal[any](t, childValue.Get(t),
				getVar(t, c.Runtime.Get(t), childID.Get(t), childKey.Get(t)),
				"precondition: the child owns the variable once it ran")

			t.Random.Repeat(3, 7, func() {
				assert.NoError(t, c.ActExecute(t))
			})

			c.ChildrenCompletionAre(t, c.ProcessID.Get(t), true)

			assert.Equal[any](t, childValue.Get(t),
				getVar(t, c.Runtime.Get(t), childID.Get(t), childKey.Get(t)),
				"replaying the spawn must not restore the parent's value over the child's")
		})
	})
}

func ExampleJoin() {
	v := "uwu"
	_ = workflow.Sequence{
		workflow.Spawn{
			Name: "deep-research",
			Definition: workflow.Sequence{
				workflow.SetVar{Name: "topic", Value: v},
				workflow.ExecuteParticipant{
					ID:     "deep-research",
					Input:  []workflow.VarName{"topic"},
					Output: []workflow.VarName{"results"},
				},
			},
		},
		workflow.Spawn{
			Name: "recent-news",
			Definition: workflow.Sequence{
				workflow.SetVar{Name: "topic", Value: v},
				workflow.ExecuteParticipant{
					ID:     "fetch-news",
					Input:  []workflow.VarName{"topic"},
					Output: []workflow.VarName{"output"},
				},
			},
		},
		workflow.Join{
			SpawnName: "deep-research",
			Collect: workflow.VarMapping{
				"results": "deep-research-result",
			},
		},
		workflow.Join{
			SpawnName: "recent-news",
			Collect: workflow.VarMapping{
				"output": "recent-news-result",
			},
		},
	}

}

func TestJoin(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	subject := let.Var(s, func(t *testcase.T) workflow.Join {
		return workflow.Join{}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return subject.Get(t)
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := c.ActExecute

		s.When("spawn(s) was used previously as part of the workflow", func(s *testcase.Spec) {
			fooPhaser := let.Phaser(s)
			barPhaser := let.Phaser(s)

			_, fooParticipant := wftest.LetParticipant(s, c, func(t *testcase.T) func(ctx context.Context) error {
				return func(ctx context.Context) error {
					fooPhaser.Get(t).Wait()
					return nil
				}
			})
			_, barParticipant := wftest.LetParticipant(s, c, func(t *testcase.T) func(ctx context.Context) error {
				return func(ctx context.Context) error {
					barPhaser.Get(t).Wait()
					return nil
				}
			})

			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					workflow.Spawn{
						Name: "foo",
						Definition: workflow.Sequence{
							workflow.SetVar{Name: "from-foo", Value: "foo-value"},
							workflow.ExecuteParticipant{ID: fooParticipant.Get(t)},
						},
					},
					workflow.Spawn{
						Name: "bar",
						Definition: workflow.Sequence{
							workflow.SetVar{Name: "from-bar", Value: "bar-value"},
							workflow.ExecuteParticipant{ID: barParticipant.Get(t)},
						},
					},
					subject.Get(t),
				}
			})

			s.And("join doesn't have any spawn name or collection specified", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) workflow.Join {
					return workflow.Join{}
				})

				s.Then("join will issue suspend signals until the sub process completed", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), workflow.Suspend{})

					fooPhaser.Get(t).Finish()
					assert.ErrorIs(t, act(t), workflow.Suspend{})

					barPhaser.Get(t).Finish()
					t.Eventually(func(t *testcase.T) {
						assert.NoError(t, act(t))
					})
				})
			})

			s.And("join specified a name", func(s *testcase.Spec) {
				name := let.Var[workflow.SpawnName](s, nil)

				subject.Let(s, func(t *testcase.T) workflow.Join {
					return workflow.Join{SpawnName: name.Get(t)}
				})

				s.And("name is referencing an unknown spawn name", func(s *testcase.Spec) {
					name.Let(s, func(t *testcase.T) workflow.SpawnName {
						return workflow.SpawnName(t.Random.UUID())
					})

					s.Then("it results in an error, stating that no sub process found by the given name", func(t *testcase.T) {
						got := act(t)
						assert.Error(t, got)
						assert.Contains(t, got.Error(), "SpawnName")
					})
				})

				s.And("name referencing a previously started spawn", func(s *testcase.Spec) {
					name.Let(s, func(t *testcase.T) workflow.SpawnName {
						return "foo"
					})

					s.And("the child finished already", func(s *testcase.Spec) {
						s.Before(func(t *testcase.T) {
							fooPhaser.Get(t).Finish()
						})

						s.Then("it eventually completes", func(t *testcase.T) {
							t.Eventually(func(t *testcase.T) {
								assert.NoError(t, act(t))
							})
						})

						s.And("Join specified collection too", func(s *testcase.Spec) {
							collect := let.Var[workflow.VarMapping](s, nil)
							subject.Let(s, func(t *testcase.T) workflow.Join {
								sub := subject.Super(t)
								sub.Collect = collect.Get(t)
								return sub
							})

							s.And("collection points to an existing variable", func(s *testcase.Spec) {
								collect.Let(s, func(t *testcase.T) workflow.VarMapping {
									return workflow.VarMapping{
										"to-foo-key": "from-foo",
									}
								})
							})
						})
					})

					s.And("the child is not finished yet", func(s *testcase.Spec) {
						// nothing to do to achieve this, since phaser is not completed

						s.Then("it will continue to yield suspend signals", func(t *testcase.T) {
							t.Random.Repeat(3, 7, func() {
								got := act(t)
								assert.ErrorIs(t, got, workflow.Suspend{})
							})
						})
					})
				})
			})

		})

		s.When("join has collection but no name configured", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) workflow.Join {
				return workflow.Join{
					Collect: random.Map(t.Random.IntBetween(1, 7), func() (workflow.VarName, workflow.VarName) {
						return workflow.VarName(t.Random.UUID()), workflow.VarName(t.Random.UUID())
					}),
				}
			})

			s.Then("it yields an error as collection without name is not supported", func(t *testcase.T) {
				got := act(t)
				assert.Error(t, got)
				assert.Contains(t, got.Error(), "SpawnName")
			})
		})

		s.When("spawn was not used", func(s *testcase.Spec) {
			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					workflow.SetVar{Name: "hello", Value: "world"},
					subject.Get(t),
				}
			})

			s.And("join doesn't have any spawn name or collection specified", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) workflow.Join {
					return workflow.Join{}
				})

				s.Then("it just continue without any issues and let the execution finish", func(t *testcase.T) {
					assert.NoError(t, act(t))

					c.ProcessCompletionIs(t, c.ProcessID.Get(t), true)
				})
			})

			s.And("join name and maybe collection specified", func(s *testcase.Spec) {
				subject.Let(s, func(t *testcase.T) workflow.Join {
					var collect workflow.VarMapping
					if t.Random.Bool() {
						collect = random.Map(t.Random.IntBetween(1, 3), func() (workflow.VarName, workflow.VarName) {
							return workflow.VarName(t.Random.UUID()), workflow.VarName(t.Random.UUID())
						})
					}
					return workflow.Join{
						SpawnName: "name",
						Collect:   collect,
					}
				})

				s.Then("it results in an error, stating that no sub process found by the given name", func(t *testcase.T) {
					got := act(t)
					assert.Error(t, got)
					assert.Contains(t, got.Error(), "missing")
				})
			})

		})
	})
}

func TestJoin_multiStagesJoin(tt *testing.T) {
	s := testcase.NewSpec(tt)
	c := wftest.LetC(s)
	t := testcase.NewTWithSpec(tt, s)

	stage1 := synckit.Phaser{}
	defer stage1.Finish()
	var after1 atomic.Bool

	stage2 := synckit.Phaser{}
	defer stage1.Finish()

	pid := mustProcessID(t)

	def := workflow.Sequence{
		workflow.Spawn{
			Name: "foo",
			Definition: wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
				stage1.Wait()
				return nil
			}},
		},
		workflow.Join{},
		wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
			after1.Store(true)
			return nil
		}},
		workflow.Spawn{
			Name: "bar",
			Definition: wftest.Stub{StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
				stage2.Wait()
				return nil
			}},
		},
		workflow.Join{},
	}

	assert.NoError(t, c.Runtime.Get(t).Bind(t.Context(), pid, def))

	act := let.Act(func(t *testcase.T) error {
		return c.Runtime.Get(t).Execute(t.Context(), pid)
	})

	// before stage1 is finished, the first Join{} is reached while foo is still blocked -> suspend
	t.Random.Repeat(3, 7, func() {
		assert.ErrorIs(t, act(t), workflow.Suspend{})
	})

	assert.False(t, after1.Load(),
		"the step between the two Joins must not have run yet, because foo is still blocked")

	// once stage1 is finished, foo completes, the first Join{} returns,
	// the stub between the Joins runs and sets after1 = true,
	// then the second Spawn+Join{} is reached, but bar is still blocked at stage2 -> suspend
	stage1.Finish()

	t.Eventually(func(t *testcase.T) {
		assert.ErrorIs(t, act(t), workflow.Suspend{})

		assert.True(t, after1.Load(),
			"the stub between the two Joins must have run",
			"proving that the first Join{} completed")
	})

	// once stage2 is finished, the second Join{} returns and the workflow finishes
	stage2.Finish()
	t.Eventually(func(t *testcase.T) {
		assert.NoError(t, act(t))
	})

	c.ProcessCompletionIs(t, pid, true)
}
