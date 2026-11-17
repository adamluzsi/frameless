package localfs_test

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/localfs"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestRepository_Workflow(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		repository = let.Var(s, func(t *testcase.T) *workflowEventRepository {
			return &workflowEventRepository{Repository: &localfs.Repository[workflow.Event, workflow.EventID]{
				Dir:     t.TempDir(),
				FileExt: "json",
				Codec:   workflowEventCodec{Codec: wfjson.NewCodec()},
			}}
		})
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			pid, err := workflow.MakeProcessID()
			assert.Must(t).NoError(err)
			return pid
		})
		id           = let.UUID(s)
		failure      = let.Error(s)
		calls        = let.Var(s, func(t *testcase.T) *atomic.Int32 { return new(atomic.Int32) })
		participants = let.Var(s, func(t *testcase.T) workflow.Participants {
			count := calls.Get(t)
			return workflow.Participants{
				workflow.ParticipantID(id.Get(t)): func(context.Context) error {
					count.Add(1)
					return failure.Get(t)
				},
			}
		})
		ctx = let.Var(s, func(t *testcase.T) context.Context {
			rt := workflow.Runtime{Events: repository.Get(t), Participants: participants.Get(t)}
			return workflow.WithName(rt.Context(t.Context()), "audit")
		})
		subject = let.Var(s, func(t *testcase.T) workflow.Execute {
			return workflow.Execute{Participant: workflow.ParticipantID(id.Get(t))}
		})
	)
	events := func(t *testcase.T) []workflow.Event {
		t.Helper()
		got, err := iterkit.CollectE(repository.Get(t).FindByProcessID(ctx.Get(t), processID.Get(t)))
		assert.NoError(t, err)
		return got
	}

	// The participant performs no writes: the executor's history read alone must
	// not prevent its failure audit from being persisted before it returns.
	s.Describe("workflow.Execute", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			d, pid, ctx := subject.Get(t), processID.Get(t), ctx.Get(t)
			return workflowCallWithin(t, ctx, func(ctx context.Context) error { return d.Execute(ctx, pid) })
		}
		assertFailure := func(t *testcase.T, event workflow.Event, failure error) {
			t.Helper()
			got := assertWorkflowFailure(t, event, processID.Get(t), workflow.Path{{Name: "audit"}, {Name: id.Get(t)}}, failure)
			assert.Equal(t, got.ParticipantID, workflow.ParticipantID(id.Get(t)))
		}

		s.Then("returns the original failure promptly and persists its audit after the read-only transaction", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), failure.Get(t))
			assert.Equal(t, calls.Get(t).Load(), int32(1))
			got := events(t)
			assert.Equal(t, len(got), 1, "only the failure audit may survive the failed attempt")
			assertFailure(t, got[0], failure.Get(t))
		})

		s.Then("audits retries independently, then caches only the successful attempt", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), failure.Get(t))
			first := events(t)
			assert.Equal(t, len(first), 1)
			assertFailure(t, first[0], failure.Get(t))

			failure.Set(t, t.Random.Error())
			assert.ErrorIs(t, act(t), failure.Get(t))
			failed := events(t)
			assert.Equal(t, len(failed), 2)
			assert.Equal(t, failed[0], first[0], "a later attempt must neither replace nor duplicate an earlier audit")
			assertFailure(t, failed[1], failure.Get(t))
			assert.NotEqual(t, failed[0].GetEventID(), failed[1].GetEventID())
			assert.Equal(t, calls.Get(t).Load(), int32(2))

			failure.Set(t, nil)
			assert.NoError(t, act(t))
			committed := events(t)
			assert.Equal(t, len(committed), 3)
			assert.Equal(t, committed[:2], failed)
			assert.Equal(t, committed[2].EventType(), (workflow.EventParticipant{}).EventType())
			assert.Equal(t, calls.Get(t).Load(), int32(3))

			assert.NoError(t, act(t))
			assert.Equal(t, calls.Get(t).Load(), int32(3), "a successful attempt is replayed, not called again")
			assert.Equal(t, events(t), committed, "replay must not flush failure audits from earlier attempts")
		})

		s.When("the call succeeds", func(s *testcase.Spec) {
			failure.LetValue(s, nil)

			s.Then("persists and replays its result through the filesystem codec", func(t *testcase.T) {
				assert.NoError(t, act(t))
				committed := events(t)
				assert.Equal(t, len(committed), 1)
				assert.Equal(t, committed[0].EventType(), (workflow.EventParticipant{}).EventType())
				assert.NoError(t, act(t))
				assert.Equal(t, calls.Get(t).Load(), int32(1))
				assert.Equal(t, events(t), committed)
			})
		})
	})

	s.Describe("workflow.Execute with a participant-returned definition", func(s *testcase.Spec) {
		var (
			prepareCalls = let.Var(s, func(t *testcase.T) *atomic.Int32 { return new(atomic.Int32) })
			failureCalls = let.Var(s, func(t *testcase.T) *atomic.Int32 { return new(atomic.Int32) })
			value        = let.UUID(s)
		)
		participants.Let(s, func(t *testcase.T) workflow.Participants {
			var (
				outer, prepare, failing = calls.Get(t), prepareCalls.Get(t), failureCalls.Get(t)
				vars                    = workflow.Vars{ProcessID: processID.Get(t), EventsRepository: repository.Get(t)}
				value                   = value.Get(t)
			)
			return workflow.Participants{
				workflow.ParticipantID(id.Get(t)): func(ctx context.Context) error {
					outer.Add(1)
					if err := vars.Set(ctx, "producer", value); err != nil {
						return err
					}
					return workflow.Sequence{
						workflow.Execute{Participant: "prepare", Output: []workflow.VarName{"prepared"}},
						workflow.Execute{Participant: "fail"},
					}
				},
				"prepare": func(context.Context) (string, error) {
					prepare.Add(1)
					return value, nil
				},
				"fail": func(ctx context.Context) error {
					failing.Add(1)
					if err := vars.Set(ctx, "failed-step", value); err != nil {
						return err
					}
					return failure.Get(t)
				},
			}
		})
		act := func(t *testcase.T) error {
			d, pid, ctx := subject.Get(t), processID.Get(t), ctx.Get(t)
			return workflowCallWithin(t, ctx, func(ctx context.Context) error { return d.Execute(ctx, pid) })
		}

		s.Then("keeps each nested failure audit outside the outer rollback without leaking calls or variables into retries", func(t *testcase.T) {
			vars := workflow.Vars{ProcessID: processID.Get(t), EventsRepository: repository.Get(t)}
			var previous []workflow.Event
			for attempt := 1; attempt <= 2; attempt++ {
				failure.Set(t, t.Random.Error())
				assert.ErrorIs(t, act(t), failure.Get(t))
				got := events(t)
				assert.Equal(t, len(got), attempt, "producer, successful child and variable events must all roll back")
				assert.Contains(t, got, previous)
				audit := assertWorkflowFailure(t, got[attempt-1], processID.Get(t), workflow.Path{
					{Name: "audit"}, {Name: id.Get(t)}, {Name: "sequence"}, {Name: "[1]"}, {Name: "fail"},
				}, failure.Get(t))
				assert.Equal(t, audit.ParticipantID, workflow.ParticipantID("fail"))
				state, err := vars.ToMap(ctx.Get(t))
				assert.NoError(t, err)
				assert.Empty(t, state)
				assert.Equal(t, calls.Get(t).Load(), int32(attempt))
				assert.Equal(t, prepareCalls.Get(t).Load(), int32(attempt), "rolled-back successes must execute again")
				assert.Equal(t, failureCalls.Get(t).Load(), int32(attempt))
				previous = got
			}
			assert.NotEqual(t, previous[0].GetEventID(), previous[1].GetEventID())

			failure.Set(t, nil)
			assert.NoError(t, act(t))
			committed := events(t)
			assert.Contains(t, committed, previous)
			var audits []workflow.EventError
			for _, event := range committed {
				if audit, ok := event.(workflow.EventError); ok {
					audits = append(audits, audit)
				}
			}
			assert.Equal(t, len(audits), 2, "successful retries must not re-flush earlier failures")
			state, err := vars.ToMap(ctx.Get(t))
			assert.NoError(t, err)
			assert.Equal(t, state, map[workflow.VarName]any{
				"producer": value.Get(t), "prepared": value.Get(t), "failed-step": value.Get(t),
			})

			assert.NoError(t, act(t))
			assert.Equal(t, events(t), committed)
			assert.Equal(t, calls.Get(t).Load(), int32(3))
			assert.Equal(t, prepareCalls.Get(t).Load(), int32(3))
			assert.Equal(t, failureCalls.Get(t).Load(), int32(3))
		})

		s.When("the follow-up initially suspends", func(s *testcase.Spec) {
			participants.Let(s, func(t *testcase.T) workflow.Participants {
				prs := participants.Super(t)
				fail := prs["fail"].(func(context.Context) error)
				count := failureCalls.Get(t)
				prs["fail"] = func(ctx context.Context) error {
					if count.Load() == 0 {
						count.Add(1)
						return workflow.Suspend{}
					}
					return fail(ctx)
				}
				return prs
			})

			s.Then("audits a nested failure during cached producer replay without losing committed progress or leaking new variables", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), workflow.Suspend{})
				committed := events(t)
				assert.NotEmpty(t, committed)
				assert.NoneOf(t, committed, func(t testing.TB, event workflow.Event) {
					assert.Equal(t, event.EventType(), (workflow.EventError{}).EventType())
				})
				vars := workflow.Vars{ProcessID: processID.Get(t), EventsRepository: repository.Get(t)}
				state, err := vars.ToMap(ctx.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, state, map[workflow.VarName]any{"producer": value.Get(t), "prepared": value.Get(t)})

				assert.ErrorIs(t, act(t), failure.Get(t))
				got := events(t)
				assert.Equal(t, len(got), len(committed)+1, "only the new failure audit may survive the enclosing rollback")
				assert.Equal(t, got[:len(committed)], committed, "previously committed success events must survive unchanged")
				audit := assertWorkflowFailure(t, got[len(committed)], processID.Get(t), workflow.Path{
					{Name: "audit"}, {Name: id.Get(t)}, {Name: "sequence"}, {Name: "[1]"}, {Name: "fail"},
				}, failure.Get(t))
				assert.Equal(t, audit.ParticipantID, workflow.ParticipantID("fail"))
				after, err := vars.ToMap(ctx.Get(t))
				assert.NoError(t, err)
				assert.Equal(t, after, state, "the failed step's variable write must not leak")
				assert.Equal(t, calls.Get(t).Load(), int32(1), "the producer must be replayed from its committed event")
				assert.Equal(t, prepareCalls.Get(t).Load(), int32(1), "the committed child success must be replayed too")
				assert.Equal(t, failureCalls.Get(t).Load(), int32(2))
			})
		})

		s.When("the returned definition succeeds", func(s *testcase.Spec) {
			failure.LetValue(s, nil)

			s.Then("replays the persisted definition and its nested successes without calling them again", func(t *testcase.T) {
				assert.NoError(t, act(t))
				committed := events(t)
				assert.NotEmpty(t, committed)
				assert.NoneOf(t, committed, func(t testing.TB, event workflow.Event) {
					assert.Equal(t, event.EventType(), (workflow.EventError{}).EventType())
				})
				assert.NoError(t, act(t))
				assert.Equal(t, events(t), committed)
				assert.Equal(t, calls.Get(t).Load(), int32(1))
				assert.Equal(t, prepareCalls.Get(t).Load(), int32(1))
				assert.Equal(t, failureCalls.Get(t).Load(), int32(1))
			})
		})
	})
}

func workflowCallWithin(t testing.TB, parent context.Context, call func(context.Context) error) (err error) {
	t.Helper()
	assert.Within(t, 2*time.Second, func(context.Context) {
		// The shorter operation deadline cleans up a blocked transaction, but
		// deadline-based unblocking must fail even if Within observes a return.
		ctx, cancel := context.WithTimeout(parent, time.Second)
		defer cancel()
		err = call(ctx)
		assert.NoError(t, ctx.Err(), "execution must return without cancellation releasing its event transaction lock")
	})
	return err
}

func assertWorkflowFailure(t testing.TB, event workflow.Event, pid workflow.ProcessID, path workflow.Path, failure error) workflow.EventError {
	t.Helper()
	audit, ok := event.(workflow.EventError)
	assert.True(t, ok, assert.MessageF("expected a failure audit, got %T", event))
	assert.Equal(t, audit.ProcessID, pid)
	assert.Equal(t, audit.Path, path)
	assert.Equal(t, audit.Error, failure.Error())
	assert.NoError(t, workflow.ValidateEventID(audit.EventID))
	assert.NotEmpty(t, audit.Timestamp)
	return audit
}

type workflowEventRepository struct {
	*localfs.Repository[workflow.Event, workflow.EventID]
}

var _ workflow.EventRepository = (*workflowEventRepository)(nil)

func (r *workflowEventRepository) FindByProcessID(ctx context.Context, pid workflow.ProcessID) iter.Seq2[workflow.Event, error] {
	return func(yield func(workflow.Event, error) bool) {
		var events []workflow.Event
		for event, err := range r.FindAll(ctx) {
			if err != nil {
				yield(nil, err)
				return
			}
			if event.GetProcessID() == pid {
				events = append(events, event)
			}
		}
		slices.SortFunc(events, func(a, b workflow.Event) int {
			if order := a.GetTimestamp().Compare(b.GetTimestamp()); order != 0 {
				return order
			}
			return a.GetEventID().Compare(b.GetEventID())
		})
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

type workflowEventCodec struct{ workflow.Codec }

func (c workflowEventCodec) Marshal(value any) ([]byte, error) {
	event, ok := value.(workflow.Event)
	if !ok {
		return nil, fmt.Errorf("expected workflow.Event, got %T", value)
	}
	// Repository passes a concrete value as any; retain the Event interface so
	// wfjson writes the type envelope needed to decode heterogeneous events.
	return c.Codec.Marshal(&event)
}
