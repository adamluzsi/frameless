package workflow_test

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wftemplate"
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

		blockingParticipantID = wftest.LetParticipantID(s)
		// eagerly loaded, so it is initialised on the test goroutine instead of the participant's, see TestRuntime_phaserLazyInitRace.
		blockingParticipantPhaser = let.Phaser(s).EagerLoading(s)
		_                         = wftest.LetParticipantWithID(s, blockingParticipantID, func(t *testcase.T) func(ctx context.Context) error {
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
				workflow.Execute{Participant: blockingParticipantID.Get(t)},
				workflow.SetVar{Name: "sub-wf-key-in-def", Value: expVal1.Get(t)},
				workflow.SetVar{Name: "sub-wf-key-in-spawn", Value: expVal2.Get(t)},
			},
		}
	})

	_, spawnerParticipantID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			return subject.Get(t)
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		if t.Random.Bool() {
			t.Log("spawn can be used as an participant signal result value too")
			return workflow.Execute{Participant: spawnerParticipantID.Get(t)}
		}
		return subject.Get(t)
	})

	s.Describe("#Execute", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) error {
			return c.ActExecute(t)
		})
		spawn := let.Act(func(t *testcase.T) error {
			return c.Runtime.Get(t).Spawn(t.Context(), c.ProcessID.Get(t), c.Definition.Get(t))
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

		s.When("the parent process remains busy after spawning", func(s *testcase.Spec) {
			var (
				parentBusy    = let.Phaser(s).EagerLoading(s)
				parentBlocked = let.VarOf(s, false)
				_, parentPAID = wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
					return func(ctx context.Context) error {
						parentBlocked.Set(t, true)
						parentBusy.Get(t).Wait()
						return nil
					}
				})
			)

			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					c.Definition.Super(t), // og spawn
					workflow.Execute{Participant: parentPAID.Get(t)},
				}
			})

			c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
				rt := c.Runtime.Super(t)
				if rt.NumQueueSubscriber < 2 {
					// ensure concurrent processing
					rt.NumQueueSubscriber = 2
				}
				return rt
			})

			s.Before(func(t *testcase.T) {
				assert.Assert(t, 1 < c.Runtime.Get(t).NumQueueSubscriber,
					"expected that num queue s")
			})

			s.Then("the spawned child is executed even though the parent is still blocked", func(t *testcase.T) {
				assert.NoError(t, spawn(t))

				t.Eventually(func(t *testcase.T) {
					assert.True(t, parentBlocked.Get(t),
						"precondition: the parent must reach its blocking participant, proving it already spawned the child")
				})

				var events = c.EventRepository.Get(t)

				t.Eventually(func(t *testcase.T) {
					assert.NotEmpty(t, iterkit.Count(wftest.IterChildren(t, events, c.ProcessID.Get(t))))
				})

				blockingParticipantPhaser.Get(t).Finish()

				for childID := range wftest.IterChildren(t, events, c.ProcessID.Get(t)) {
					assert.Eventually(t, deadline, func(t testing.TB) {
						ok, err := workflow.IsCompleted(t.Context(), events, childID)
						assert.NoError(t, err)
						assert.Assert(t, ok, "child should finish up eventually, while the parent is still busy")
					})
				}
			})
		})

		s.When("the Spawn is in the body of a loop", func(s *testcase.Spec) {
			// for _, item := range items {
			// 	go child(item) // every round starts a goroutine of its own
			// }
			var (
				items = let.Var(s, func(t *testcase.T) []string {
					return random.Slice(t.Random.IntBetween(2, 5), t.Random.UUID)
				})
				suspended      = let.VarOf(s, false)
				_, suspendOnce = wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
					return func(ctx context.Context) error {
						if suspended.Get(t) {
							return nil
						}
						suspended.Set(t, true)
						return workflow.Suspend{}
					}
				})
			)
			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					workflow.SetVar{Name: "items", Value: items.Get(t)},
					workflow.ForEach{
						Over: "items",
						V:    "item",
						Do: workflow.Spawn{
							Name:       subject.Get(t).Name,
							Definition: workflow.Sequence{},
							Vars:       workflow.VarMapping{"item": "item"},
						},
					},
					// The Process suspends after the loop, so resuming it replays the loop.
					workflow.Execute{Participant: suspendOnce.Get(t)},
				}
			})
			// spawned lists the item each child got, in the order the children were spawned.
			spawned := func(t *testcase.T) []any {
				t.Helper()
				var got []any
				for _, event := range c.ProcessEvents(t, c.ProcessID.Get(t)) {
					if spawn, ok := event.(workflow.EventSpawn); ok {
						got = append(got, getVar(t, c.Runtime.Get(t), spawn.ChildID, "item"))
					}
				}
				return got
			}
			expected := func(t *testcase.T) []any {
				return slicekit.Map(items.Get(t), func(item string) any { return item })
			}

			s.Then("every round spawns a child of its own, which gets the item of its round", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})

				assert.Equal(t, expected(t), spawned(t))
			})

			s.Then("resuming the Process replays the loop, rather than spawning the children of its rounds again", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				t.Eventually(func(t *testcase.T) {
					assert.NoError(t, act(t))
				})
				c.ProcessCompletionIs(t, c.ProcessID.Get(t), true)

				assert.Equal(t, expected(t), spawned(t))
			})
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

	s.When("the Spawn is in a variable scope of its own, like the body of a loop", func(s *testcase.Spec) {
		// for _, parentKey := range []string{parentValue} {
		// 	go child(parentKey) // the child receives it as childKey
		// }
		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.SetVar{Name: "items", Value: []string{parentValue.Get(t)}},
				workflow.ForEach{
					Over: "items", V: parentKey.Get(t),
					Do: workflow.Spawn{
						Name:       workflow.SpawnName(t.Random.UUID()),
						Definition: workflow.SetVar{Name: "sentinel", Value: "child-finished"},
						Vars:       workflow.VarMapping{parentKey.Get(t): childKey.Get(t)},
					},
				},
			}
		})

		s.Then("the forwarded variables are declared in the root scope of the child, where the child can see them", func(t *testcase.T) {
			assert.NoError(t, c.ActExecute(t))
			c.WaitForSpawn(t, c.ProcessID.Get(t))

			assert.Equal[any](t, parentValue.Get(t),
				getVar(t, c.Runtime.Get(t), childID.Get(t), childKey.Get(t)),
				"the child doesn't run in the variable scope of its parent's loop body,",
				"so the variables it is given can't be kept there either")
		})
	})

	s.When("the event log fails while the variables are forwarded to the child", func(s *testcase.Spec) {
		// go child(parentKey) // a goroutine never starts without its arguments
		var (
			expErr       = let.Error(s)
			forwardFails = let.Var(s, func(t *testcase.T) *atomic.Bool {
				var fails atomic.Bool
				fails.Store(true)
				return &fails
			})
		)
		c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
			forwarded := childKey.Get(t)
			rt := c.Runtime.Super(t)
			rt.Events = failingEventRepository{
				WorkflowEventRepository: c.EventRepository.Get(t),
				Err:                     expErr.Get(t),
				Fails:                   forwardFails.Get(t),
				FailsOn: func(event workflow.Event) bool {
					set, ok := event.(workflow.EventSetVar)
					return ok && set.Name == forwarded
				},
			}
			return rt
		})

		s.Then("the child is spawned with its variables once the event log recovers", func(t *testcase.T) {
			assert.ErrorIs(t, c.ActExecute(t), expErr.Get(t))

			forwardFails.Get(t).Store(false)
			assert.NoError(t, c.ActExecute(t))
			c.WaitForSpawn(t, c.ProcessID.Get(t))
			c.ChildrenCompletionAre(t, c.ProcessID.Get(t), true)

			assert.Equal[any](t, parentValue.Get(t),
				getVar(t, c.Runtime.Get(t), childID.Get(t), childKey.Get(t)),
				"the retried Spawn must not take a child left without its variables for an already spawned one")
		})

		s.Then("the Spawn is not recorded either, so it is retried as a whole", func(t *testcase.T) {
			assert.ErrorIs(t, c.ActExecute(t), expErr.Get(t))

			for _, event := range c.ProcessEvents(t, c.ProcessID.Get(t)) {
				_, ok := event.(workflow.EventSpawn)
				assert.False(t, ok, "the Spawn is only recorded together with the variables forwarded to the child")
			}
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
				workflow.Execute{
					Participant: "deep-research",
					Input:       []workflow.VarName{"topic"},
					Output:      []workflow.VarName{"results"},
				},
			},
		},
		workflow.Spawn{
			Name: "recent-news",
			Definition: workflow.Sequence{
				workflow.SetVar{Name: "topic", Value: v},
				workflow.Execute{
					Participant: "fetch-news",
					Input:       []workflow.VarName{"topic"},
					Output:      []workflow.VarName{"output"},
				},
			},
		},
		workflow.Join{
			Spawn: "deep-research",
			Collect: workflow.VarMapping{
				"results": "deep-research-result",
			},
		},
		workflow.Join{
			Spawn: "recent-news",
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
			// eagerly loaded, so they are initialised on the test goroutine instead of the participants', see TestRuntime_phaserLazyInitRace.
			fooPhaser := let.Phaser(s).EagerLoading(s)
			barPhaser := let.Phaser(s).EagerLoading(s)

			_, fooParticipant := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
				return func(ctx context.Context) error {
					fooPhaser.Get(t).Wait()
					return nil
				}
			})
			_, barParticipant := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
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
							workflow.Execute{Participant: fooParticipant.Get(t)},
						},
					},
					workflow.Spawn{
						Name: "bar",
						Definition: workflow.Sequence{
							workflow.SetVar{Name: "from-bar", Value: "bar-value"},
							workflow.Execute{Participant: barParticipant.Get(t)},
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
					return workflow.Join{Spawn: name.Get(t)}
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

		// The Join collects the results of a child, the way a Go function returns them to its caller:
		//
		//	child := func() (result string) { result = "..."; return }
		//	parentKey := child() // Join{Spawn: "child", Collect: {result: parentKey}}
		//	see(parentKey)
		s.When("the Join collects variables from the joined child", func(s *testcase.Spec) {
			var (
				childKey    = letVarName(s)
				parentKey   = letVarName(s)
				unlistedKey = letVarName(s)
				childValue  = let.String(s)

				seen = let.VarOf[[]any](s, nil)

				// child is what the joined child runs.
				child = let.Var(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						workflow.SetVar{Name: childKey.Get(t), Value: childValue.Get(t)},
						workflow.SetVar{Name: unlistedKey.Get(t), Value: t.Random.String()},
					}
				})
				// before is what the parent runs before spawning the child.
				before = let.Var(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{}
				})
				// spawn is where the parent spawns the child.
				spawn = let.Var(s, func(t *testcase.T) workflow.Definition {
					return workflow.Spawn{Name: "child", Definition: child.Get(t)}
				})
				// join is where the parent joins the child, which is right after spawning it.
				join = let.Var(s, func(t *testcase.T) workflow.Definition {
					return subject.Get(t)
				})
				// after is what the parent runs once it joined the child.
				after = let.Var(s, func(t *testcase.T) workflow.Definition {
					return peekVar(t, seen, parentKey.Get(t))
				})
			)
			subject.Let(s, func(t *testcase.T) workflow.Join {
				return workflow.Join{
					Spawn:   "child",
					Collect: workflow.VarMapping{childKey.Get(t): parentKey.Get(t)},
				}
			})
			c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
				return workflow.Sequence{
					before.Get(t),
					spawn.Get(t),
					join.Get(t),
					after.Get(t),
				}
			})
			// joined executes the parent until it is completed,
			// which takes the child to be completed and joined first.
			joined := func(t *testcase.T) {
				t.Helper()
				t.Eventually(func(t *testcase.T) {
					assert.NoError(t, act(t))
				})
				c.ProcessCompletionIs(t, c.ProcessID.Get(t), true)
			}

			s.Then("the child variable is assigned to the parent variable it is mapped to", func(t *testcase.T) {
				joined(t)

				assert.Equal(t, []any{childValue.Get(t)}, seen.Get(t),
					"the steps after the Join see the collected value")
				assert.Equal[any](t, childValue.Get(t),
					getVar(t, c.Runtime.Get(t), c.ProcessID.Get(t), parentKey.Get(t)))
			})

			s.Then("only the listed variables are collected, under the names they are mapped to", func(t *testcase.T) {
				joined(t)

				vars := getVars(t, c.Runtime.Get(t), c.ProcessID.Get(t))
				for _, name := range []workflow.VarName{childKey.Get(t), unlistedKey.Get(t)} {
					_, ok, err := vars.Lookup(t.Context(), name)
					assert.NoError(t, err)
					assert.False(t, ok, assert.MessageF("%s is not expected to be in the parent", name))
				}
			})

			s.When("the child doesn't set the collected variable", func(s *testcase.Spec) {
				child.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.SetVar{Name: unlistedKey.Get(t), Value: t.Random.String()}
				})

				s.Then("it is skipped, so the parent variable is not declared either", func(t *testcase.T) {
					joined(t)

					assert.Equal(t, []any{undeclared}, seen.Get(t))
				})

				s.And("the parent assigned the variable before the Join", func(s *testcase.Spec) {
					parentValue := let.String(s)
					before.Let(s, func(t *testcase.T) workflow.Definition {
						return workflow.SetVar{Name: parentKey.Get(t), Value: parentValue.Get(t)}
					})

					s.Then("the parent value remains, like a default", func(t *testcase.T) {
						joined(t)

						assert.Equal(t, []any{parentValue.Get(t)}, seen.Get(t))
					})
				})
			})

			s.When("the child sets the collected variable only in a variable scope nested in its root", func(s *testcase.Spec) {
				// go func() {
				// 	if true {
				// 		childKey := childValue
				// 	}
				// }()
				child.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.If{
						Cond: wftemplate.Condition("true"),
						Then: workflow.SetVar{Name: childKey.Get(t), Value: childValue.Get(t)},
					}
				})

				s.Then("it is not collected, as the variable is gone by the time the child completes", func(t *testcase.T) {
					joined(t)

					assert.Equal(t, []any{undeclared}, seen.Get(t))
				})
			})

			s.When("the Join is in a variable scope of its own, like a branch of an If", func(s *testcase.Spec) {
				// if true {
				// 	parentKey := child()
				// 	see(parentKey)
				// }
				// see(parentKey)
				join.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.If{
						Cond: wftemplate.Condition("true"),
						Then: workflow.Sequence{
							subject.Get(t),
							peekVar(t, seen, parentKey.Get(t)),
						},
					}
				})

				s.Then("the collected variable is declared in that scope, so it is gone after it", func(t *testcase.T) {
					joined(t)

					assert.Equal(t, []any{childValue.Get(t), undeclared}, seen.Get(t))
				})

				s.And("the parent variable is declared outside of that scope", func(s *testcase.Spec) {
					// parentKey := parentValue
					// if true {
					// 	parentKey = child()
					// 	see(parentKey)
					// }
					// see(parentKey)
					before.Let(s, func(t *testcase.T) workflow.Definition {
						return workflow.SetVar{Name: parentKey.Get(t), Value: t.Random.String()}
					})

					s.Then("the collected value is assigned to the outer variable, like any assignment would be", func(t *testcase.T) {
						joined(t)

						assert.Equal(t, []any{childValue.Get(t), childValue.Get(t)}, seen.Get(t))
					})
				})
			})

			s.When("the Spawn is in a variable scope of its own, like a branch of an If", func(s *testcase.Spec) {
				// if true {
				// 	go child()
				// }
				// parentKey := child() // the Join after the If
				// see(parentKey)
				spawn.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.If{
						Cond: wftemplate.Condition("true"),
						Then: spawn.Super(t),
					}
				})

				s.Then("the Join after it still finds the child, as the only one spawned by that name", func(t *testcase.T) {
					joined(t)

					assert.Equal(t, []any{childValue.Get(t)}, seen.Get(t))
				})

				s.And("a child of the same name is spawned before, outside of that scope", func(s *testcase.Spec) {
					// first := child() // the child sets outerValue
					// if true {
					// 	first := child() // shadows the outer one, but only within the If
					// }
					// parentKey := first // the Join after the If
					// see(parentKey)
					outerValue := let.Var(s, func(t *testcase.T) string {
						return random.Unique(t.Random.String, childValue.Get(t))
					})
					before.Let(s, func(t *testcase.T) workflow.Definition {
						return workflow.Spawn{
							Name:       "child",
							Definition: workflow.SetVar{Name: childKey.Get(t), Value: outerValue.Get(t)},
						}
					})

					s.Then("the Join joins the outer one, as the one in the If is local to it, even if it is the latest", func(t *testcase.T) {
						joined(t)

						assert.Equal(t, []any{outerValue.Get(t)}, seen.Get(t))
					})
				})
			})

			s.When("the Join is deferred, and none of the Spawns of its name is visible to it", func(s *testcase.Spec) {
				// if true {
				// 	if true {
				// 		go child() // the child sets childValue
				// 	}
				// 	defer func() {
				// 		parentKey := child() // the Join, which runs once the Process ended
				// 		see(parentKey)
				// 	}()
				// }
				// if true {
				// 	go child() // the child sets otherValue, spawned the latest, but farther from the Join
				// }
				otherValue := let.Var(s, func(t *testcase.T) string {
					return random.Unique(t.Random.String, childValue.Get(t))
				})
				c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						workflow.If{
							Cond: wftemplate.Condition("true"),
							Then: workflow.Sequence{
								workflow.If{Cond: wftemplate.Condition("true"), Then: spawn.Get(t)},
								workflow.Defer{Definition: workflow.Sequence{join.Get(t), after.Get(t)}},
							},
						},
						workflow.If{
							Cond: wftemplate.Condition("true"),
							Then: workflow.Spawn{
								Name:       "child",
								Definition: workflow.SetVar{Name: childKey.Get(t), Value: otherValue.Get(t)},
							},
						},
					}
				})

				s.Then("it joins the child of the Spawn sharing the longest Path with it, rather than the latest", func(t *testcase.T) {
					joined(t)

					assert.Equal(t, []any{childValue.Get(t)}, seen.Get(t))
				})
			})

			s.When("the Spawn and the Join are in the body of a loop", func(s *testcase.Spec) {
				// for _, item := range items {
				// 	parentKey := child(item) // the child gives back what it got
				// 	see(parentKey)
				// }
				var (
					items = let.Var(s, func(t *testcase.T) []string {
						return random.Slice(t.Random.IntBetween(2, 5), t.Random.UUID)
					})
					saw    = let.VarOf[[]string](s, nil)
					_, see = wftest.LetParticipant(s, func(t *testcase.T) func(context.Context, string) error {
						return func(_ context.Context, v string) error {
							testcase.Append(t, saw, v)
							return nil
						}
					})
				)
				child.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{} // the item it got is forwarded as childKey, which the Join collects
				})
				var (
					loop = let.Var(s, func(t *testcase.T) workflow.Definition {
						return workflow.ForEach{
							Over: "items",
							V:    "item",
							Do: workflow.Sequence{
								workflow.Spawn{
									Name:       "child",
									Definition: child.Get(t),
									Vars:       workflow.VarMapping{"item": childKey.Get(t)},
								},
								join.Get(t),
								// A participant call is recorded, so the rounds replayed while a later Join waits don't see again.
								workflow.Execute{Participant: see.Get(t), Input: []workflow.VarName{parentKey.Get(t)}},
							},
						}
					})
					// seeAfterLoop is the see(parentKey) after the loop.
					seeAfterLoop = let.Var(s, func(t *testcase.T) workflow.Definition {
						return workflow.Execute{Participant: see.Get(t), Input: []workflow.VarName{parentKey.Get(t)}}
					})
				)
				c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						workflow.SetVar{Name: "items", Value: items.Get(t)},
						loop.Get(t),
					}
				})

				s.Then("every round joins the child it spawned, and collects from it", func(t *testcase.T) {
					joined(t)

					assert.Equal(t, items.Get(t), saw.Get(t))
				})

				s.And("the child is joined after the loop too", func(s *testcase.Spec) {
					// for _, item := range items {
					// 	parentKey := child(item)
					// 	see(parentKey)
					// }
					// parentKey := child() // the Join after the loop
					// see(parentKey)
					c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
						return workflow.Sequence{
							workflow.SetVar{Name: "items", Value: items.Get(t)},
							loop.Get(t),
							join.Get(t),
							seeAfterLoop.Get(t),
						}
					})

					s.Then("it joins the child of the last round, as no child of that name is spawned outside of the loop", func(t *testcase.T) {
						joined(t)

						exp := append(slicekit.Clone(items.Get(t)), items.Get(t)[len(items.Get(t))-1])
						assert.Equal(t, exp, saw.Get(t))
					})
				})

				s.And("a child of the same name is spawned before the loop, and joined after it", func(s *testcase.Spec) {
					// first := child(firstValue)
					// for _, item := range items {
					// 	parentKey := child(item)
					// 	see(parentKey)
					// }
					// parentKey := first // the Join after the loop
					// see(parentKey)
					firstValue := let.UUID(s)
					c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
						return workflow.Sequence{
							workflow.SetVar{Name: "items", Value: items.Get(t)},
							workflow.SetVar{Name: "first", Value: firstValue.Get(t)},
							workflow.Spawn{
								Name:       "child",
								Definition: child.Get(t),
								Vars:       workflow.VarMapping{"first": childKey.Get(t)},
							},
							loop.Get(t),
							join.Get(t),
							seeAfterLoop.Get(t),
						}
					})

					s.Then("the rounds join their own child, while the Join after the loop joins the one spawned before it", func(t *testcase.T) {
						joined(t)

						exp := append(slicekit.Clone(items.Get(t)), firstValue.Get(t))
						assert.Equal(t, exp, saw.Get(t))
					})
				})
			})

			s.When("the parent reassigns the collected variable after the Join, and gets resumed later", func(s *testcase.Spec) {
				var (
					reassigned = let.Var(s, func(t *testcase.T) string {
						return random.Unique(t.Random.String, childValue.Get(t))
					})
					suspended      = let.VarOf(s, false)
					_, suspendOnce = wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
						return func(ctx context.Context) error {
							if suspended.Get(t) {
								return nil
							}
							suspended.Set(t, true)
							return workflow.Suspend{}
						}
					})
				)
				after.Let(s, func(t *testcase.T) workflow.Definition {
					return workflow.Sequence{
						workflow.SetVar{Name: parentKey.Get(t), Value: reassigned.Get(t)},
						workflow.Execute{Participant: suspendOnce.Get(t)},
						peekVar(t, seen, parentKey.Get(t)),
					}
				})

				s.Then("the resumed parent replays the Join, rather than collecting the child's value over the reassigned one", func(t *testcase.T) {
					joined(t)
					assert.True(t, suspended.Get(t), "precondition: the parent was resumed after the Join")

					assert.Equal(t, []any{reassigned.Get(t)}, seen.Get(t))
					assert.Equal[any](t, reassigned.Get(t),
						getVar(t, c.Runtime.Get(t), c.ProcessID.Get(t), parentKey.Get(t)))
				})
			})

			s.When("the event log fails to record the Join", func(s *testcase.Spec) {
				var (
					expErr    = let.Error(s)
					joinFails = let.Var(s, func(t *testcase.T) *atomic.Bool {
						var fails atomic.Bool
						fails.Store(true)
						return &fails
					})
				)
				c.Runtime.Let(s, func(t *testcase.T) workflow.Runtime {
					rt := c.Runtime.Super(t)
					rt.Events = failingEventRepository{
						WorkflowEventRepository: c.EventRepository.Get(t),
						Err:                     expErr.Get(t),
						Fails:                   joinFails.Get(t),
						FailsOn: func(event workflow.Event) bool {
							_, ok := event.(workflow.EventJoin)
							return ok
						},
					}
					return rt
				})

				s.Then("the collected values are not recorded either, so the Join is retried as a whole", func(t *testcase.T) {
					t.Eventually(func(t *testcase.T) {
						assert.ErrorIs(t, act(t), expErr.Get(t))
					})
					for _, event := range c.ProcessEvents(t, c.ProcessID.Get(t)) {
						if set, ok := event.(workflow.EventSetVar); ok {
							assert.NotEqual(t, parentKey.Get(t), set.Name,
								"the collected value is only recorded together with the Join")
						}
					}

					joinFails.Get(t).Store(false)
					joined(t)

					assert.Equal(t, []any{childValue.Get(t)}, seen.Get(t))
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
						Spawn:   "name",
						Collect: collect,
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

	// A Join is a step of its own, so once it joined, a resumed execution replays it,
	// rather than joining the same children again.
	s.Context("as part of a Process which is resumed after its joins", func(s *testcase.Spec) {
		suspended := let.VarOf(s, false)
		_, suspendOnce := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				if suspended.Get(t) {
					return nil
				}
				suspended.Set(t, true)
				return workflow.Suspend{}
			}
		})
		c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
			return workflow.Sequence{
				workflow.Spawn{Name: "foo", Definition: workflow.SetVar{Name: "from-foo", Value: "foo-value"}},
				workflow.Spawn{Name: "bar", Definition: workflow.SetVar{Name: "from-bar", Value: "bar-value"}},
				workflow.Join{Spawn: "foo"},
				workflow.Join{},
				workflow.Execute{Participant: suspendOnce.Get(t)},
			}
		})
		joinsOf := func(t *testcase.T) []workflow.EventJoin {
			t.Helper()
			var joins []workflow.EventJoin
			for _, e := range c.ProcessEvents(t, c.ProcessID.Get(t)) {
				if j, ok := e.(workflow.EventJoin); ok {
					joins = append(joins, j)
				}
			}
			return joins
		}
		s.Before(func(t *testcase.T) {
			t.Eventually(func(t *testcase.T) {
				assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{})
				assert.True(t, suspended.Get(t), "the joins are waiting on the children to complete")
			})
		})

		act := func(t *testcase.T) error {
			return c.Runtime.Get(t).Execute(t.Context(), c.ProcessID.Get(t))
		}

		s.Then("the resumed execution replays the joins, rather than recording them again", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, len(joinsOf(t)), 2)
			c.ProcessCompletionIs(t, c.ProcessID.Get(t), true)
		})

		s.Then("a join records where it joined: every child under a join segment, and a named spawn under join/<name>", func(t *testcase.T) {
			assert.NoError(t, act(t))

			joins := joinsOf(t)
			assert.Equal(t, len(joins), 2)
			assert.True(t, 2 <= len(joins[0].Path))
			assert.Equal(t, joins[0].Path[len(joins[0].Path)-2:], pathOf("join", "foo"))
			assert.Equal(t, len(joins[0].Children), 1)
			assert.True(t, 1 <= len(joins[1].Path))
			assert.Equal(t, joins[1].Path[len(joins[1].Path)-1:], pathOf("join"))
			assert.Equal(t, len(joins[1].Children), 2)
		})
	})
}

func letVarName(s *testcase.Spec) testcase.Var[workflow.VarName] {
	return let.Var(s, func(t *testcase.T) workflow.VarName {
		return workflow.VarName(t.Random.UUID())
	})
}

// failingEventRepository simulates an event log which goes down right as a certain event is recorded:
// while it Fails, every event that FailsOn matches fails to be created with Err.
type failingEventRepository struct {
	*memory.WorkflowEventRepository
	Err     error
	Fails   *atomic.Bool
	FailsOn func(workflow.Event) bool
}

func (r failingEventRepository) Create(ctx context.Context, ptr *workflow.Event) error {
	if r.Fails.Load() && r.FailsOn(*ptr) {
		return r.Err
	}
	return r.WorkflowEventRepository.Create(ctx, ptr)
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

// TestSpawn_TerminateCascade asserts that calling rt.Terminate against a
// parent Process cascades the termination down to its spawned children, so
// the entire sub-workflow tree is unwound by a single external Terminate
// call rather than leaving busy children running.
//
// Scenario:
//   - The parent definition Spawns a child and then Joins on it.
//   - The child's only step is a blocking participant (phaser.Wait), so the
//     child stays busy while the parent reaches the Join and Suspends.
//   - rt.Terminate(ctx, parentPID) is then called from outside.
//
// Expectation:
//   - The parent is marked terminated (EventTerminated recorded).
//   - The child is also marked terminated, without the test needing to drive
//     the child's blocking participant to completion first.
//   - The child never reaches its natural completion (EventCompleted), so
//     the log distinguishes "called off" from "finished".
func TestTerminate_cascadeSpawn(t *testing.T) {
	s := testcase.NewSpec(t)
	c := wftest.LetC(s)

	_, blockingParticipantID := wftest.LetParticipant(s, func(t *testcase.T) func(ctx context.Context) error {
		return func(ctx context.Context) error {
			select {
			case <-ctx.Done(): // waits until cancellation
			case <-t.Done():
			}
			return nil
		}
	})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.Spawn{
				Name: workflow.SpawnName(t.Random.UUID()),
				Definition: workflow.Sequence{
					workflow.Execute{Participant: blockingParticipantID.Get(t)},
				},
			},
			workflow.Join{},
		}
	})

	s.Before(func(t *testcase.T) {
		assert.ErrorIs(t, c.ActExecute(t), workflow.Suspend{}, assert.MessageF(
			"arrangement: the parent must reach Join and Suspend while "+
				"the child is still busy on its blocking participant"))
		c.WaitForSpawn(t, c.ProcessID.Get(t))
	})

	// act: terminate the parent from outside.
	act := let.Act(func(t *testcase.T) error {
		return c.Runtime.Get(t).Terminate(t.Context(), c.ProcessID.Get(t))
	})

	s.Then("the parent is marked terminated", func(t *testcase.T) {
		assert.NoError(t, act(t))

		t.Eventually(func(t *testcase.T) {
			isTerminated, err := workflow.IsTerminated(t.Context(),
				c.EventRepository.Get(t), c.ProcessID.Get(t))
			assert.NoError(t, err)
			assert.True(t, isTerminated, assert.MessageF(
				"after rt.Terminate, the parent's event history must report the "+
					"Process as called off"))
		})
	})

	s.Then("the spawned child is also terminated", func(t *testcase.T) {
		assert.NoError(t, act(t))

		for childID := range wftest.IterChildren(t, c.EventRepository.Get(t), c.ProcessID.Get(t)) {
			t.Eventually(func(t *testcase.T) {

				isTerminated, err := workflow.IsTerminated(t.Context(),
					c.EventRepository.Get(t), childID)
				assert.NoError(t, err)
				assert.True(t, isTerminated, assert.MessageF(
					"a single rt.Terminate against the parent must cascade to the "+
						"spawned child so the caller does not need to walk the spawn "+
						"tree by hand; the child's event history must report it as "+
						"called off without first driving its blocking participant"))
			})
		}
	})
}
