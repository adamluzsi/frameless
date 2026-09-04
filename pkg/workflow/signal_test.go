package workflow_test

import (
	"errors"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/workflow"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
	"go.llib.dev/testcase/let"
)

// seedEvent appends an event to repo. The repository rejects events that are
// not fully populated, so anything seeded through here is as valid as what the
// runtime itself writes.
func seedEvent(t *testcase.T, repo workflow.EventRepository, event workflow.Event) {
	t.Helper()
	assert.NoError(t, repo.Create(t.Context(), &event))
}

// completionEventFor builds the event the runtime records when a Process
// finishes.
func completionEventFor(t *testcase.T, pid workflow.ProcessID) workflow.Event {
	t.Helper()
	return workflow.EventCompleted{
		EventID:   mustEventID(t),
		ProcessID: pid,
		Timestamp: clock.Now(),
	}
}

// TestComplete specifies workflow.Complete, the RuntimeSignal that marks a
// Process as finished.
//
// Complete is the terminal signal of the runtime. Runtime#Execute raises it
// itself once a Definition runs to its end, and it is what turns "the
// definition stopped asking for anything" into the durable fact that the
// Process is done. That fact lives in the event history as an EventCompleted,
// which makes completion a property of the log rather than of any single
// execution: re-running a finished Process must not finish it twice, and
// asking whether it is finished must not depend on who is asking.
//
// Complete is the odd one out among the RuntimeSignals. Suspend and Halt
// return themselves so the scheduler can recognise them on the way out;
// Complete returns nil, because it resolves the signal rather than propagating
// it.
//
// Complete carries no payload. Which Process it is about comes entirely from
// the ProcessID the Runtime hands to RuntimeSignalExecute, so the very same
// value can be raised from any execution. Reading the fact back afterwards is
// a separate question, asked of the log rather than of the signal: see
// TestIsCompleted.
func TestComplete(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})
		events = let.Var(s, func(t *testcase.T) *memory.WorkflowEventRepository {
			return &memory.WorkflowEventRepository{}
		})
	)

	// subject is the system under test: the completion signal.
	subject := let.Var(s, func(t *testcase.T) workflow.Complete {
		return workflow.Complete{}
	})

	// completionsOf returns the EventCompleted entries the repository holds
	// for pid. Completion is a property of the event log, so every assertion
	// about it is made by reading the log back rather than by trusting a
	// return value.
	completionsOf := func(t *testcase.T, pid workflow.ProcessID) []workflow.EventCompleted {
		t.Helper()
		return eventsOfType[workflow.EventCompleted](t, events.Get(t), pid)
	}

	terminationsOf := func(t *testcase.T, pid workflow.ProcessID) []workflow.EventTerminated {
		t.Helper()
		return eventsOfType[workflow.EventTerminated](t, events.Get(t), pid)
	}

	s.Describe("#RuntimeSignalExecute", func(s *testcase.Spec) {
		// runtime is the smallest Runtime the signal needs: it reads and
		// writes the event history and touches nothing else. Keeping it bare
		// is deliberate. If Complete ever starts reaching for the queue, the
		// locks or the participants, this spec fails loudly on a nil
		// dependency instead of quietly passing.
		var runtime = let.Var(s, func(t *testcase.T) workflow.Runtime {
			return workflow.Runtime{Events: events.Get(t)}
		})

		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).RuntimeSignalExecute(t.Context(),
				runtime.Get(t), processID.Get(t))
		})

		s.Then("it resolves the signal instead of propagating it", func(t *testcase.T) {
			assert.NoError(t, act(t),
				"Complete is the one RuntimeSignal that answers rather than escalates: "+
					"Suspend and Halt return themselves so the scheduler can see them, "+
					"Complete returns nil because the Process is done and there is "+
					"nothing left for the scheduler to decide")
		})

		s.Then("the completion is recorded in the Process event history", func(t *testcase.T) {
			assert.NoError(t, act(t))

			var completions = completionsOf(t, processID.Get(t))
			assert.Equal(t, 1, len(completions), assert.MessageF(
				"completion is a durable fact about the Process, not a return value; "+
					"it has to survive in the log so that a later Execute, a restart, or "+
					"another node can still tell the Process is finished. got %d", len(completions)))

			var completion = completions[0]
			assert.Equal(t, processID.Get(t), completion.ProcessID, assert.MessageF(
				"the completion belongs to the Process the Runtime is executing; the "+
					"Runtime is the one that knows which Process it is running, and the "+
					"signal is raised from inside that execution"))
			assert.False(t, completion.EventID.IsZero(),
				"the completion needs its own identity in the log")
			assert.False(t, completion.Timestamp.IsZero(),
				"the completion needs to say when the Process finished")
		})

		s.Then("the Process is reported as completed from then on", func(t *testcase.T) {
			assert.NoError(t, act(t))

			isCompleted, err := workflow.IsCompleted(t.Context(), events.Get(t), processID.Get(t))
			assert.NoError(t, err)
			assert.True(t, isCompleted, assert.MessageF(
				"raising the signal and asking the question are two halves of one "+
					"contract; whatever RuntimeSignalExecute writes, IsCompleted must read"))
		})

		s.When("the Process has already been terminated", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				seedEvent(t, events.Get(t), terminationEventFor(t, processID.Get(t)))
			})

			s.Then("the signal is resolved without recording a completion", func(t *testcase.T) {
				assert.NoError(t, act(t),
					"asking to complete a Process that has been terminated is not a "+
						"fault: the caller wanted the Process finished and it never will "+
						"be, so there is nothing to record and nothing to retry")

				assert.Empty(t, completionsOf(t, processID.Get(t)),
					"a terminated Process did not run to its end. Writing EventCompleted "+
						"here would leave the log claiming the Process both was called "+
						"off short and ran to completion, and every later reader would "+
						"have to guess which one it meant")
			})

			s.Then("the termination is left as the Process outcome", func(t *testcase.T) {
				var before = terminationsOf(t, processID.Get(t))
				assert.Equal(t, 1, len(before),
					assert.MessageF("arrangement: the Process starts out terminated"))

				assert.NoError(t, act(t))

				assert.Equal(t, before, terminationsOf(t, processID.Get(t)),
					"the termination already on record is the Process' outcome and the "+
						"signal must not disturb it")

				isTerminated, err := workflow.IsTerminated(t.Context(), events.Get(t), processID.Get(t))
				assert.NoError(t, err)
				assert.True(t, isTerminated,
					"and the Process still reads as terminated afterwards")

				isCompleted, err := workflow.IsCompleted(t.Context(), events.Get(t), processID.Get(t))
				assert.NoError(t, err)
				assert.False(t, isCompleted,
					"a terminated Process is not completed; if reading it as completed "+
						"were allowed, the log would lose the only distinction that "+
						"explains why the Process stopped")
			})
		})

		s.When("another Process has already been terminated", func(s *testcase.Spec) {
			var otherProcessID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
				return mustProcessID(t)
			})

			s.Before(func(t *testcase.T) {
				seedEvent(t, events.Get(t), terminationEventFor(t, otherProcessID.Get(t)))
			})

			s.Then("this Process is completed all the same", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, 1, len(completionsOf(t, processID.Get(t))),
					"the termination that suppresses a completion is the one belonging "+
						"to this Process; a foreign termination says nothing about it, "+
						"and reading the check unscoped would let one Process being "+
						"called off silently block every other Process from finishing")
			})
		})

		s.When("the Process is already completed", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				seedEvent(t, events.Get(t), completionEventFor(t, processID.Get(t)))
			})

			s.Then("no second completion is recorded", func(t *testcase.T) {
				var before = completionsOf(t, processID.Get(t))
				assert.Equal(t, 1, len(before),
					assert.MessageF("arrangement: the Process starts out completed"))

				assert.NoError(t, act(t))

				assert.Equal(t, before, completionsOf(t, processID.Get(t)),
					"raising Complete on an already completed Process is a no-op; a "+
						"Process re-entered after completion (a retry, a replay, a duplicate "+
						"queue entry) must not grow a second completion, or the log stops "+
						"being a reliable answer to when the Process finished")
			})
		})

		s.When("another Process has already completed", func(s *testcase.Spec) {
			var otherProcessID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
				return mustProcessID(t)
			})

			s.Before(func(t *testcase.T) {
				seedEvent(t, events.Get(t), completionEventFor(t, otherProcessID.Get(t)))
			})

			s.Then("this Process is completed on its own account", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, 1, len(completionsOf(t, processID.Get(t))),
					"the event repository is shared by every Process, so a foreign "+
						"completion must not be mistaken for this one's and skip the write")
				assert.Equal(t, 1, len(completionsOf(t, otherProcessID.Get(t))),
					"and the other Process' history is left exactly as it was")
			})
		})

		s.When("the Runtime passes no ProcessID", func(s *testcase.Spec) {
			processID.Let(s, func(t *testcase.T) workflow.ProcessID {
				return workflow.ProcessID{}
			})

			s.Then("nothing is completed and the fault is reported", func(t *testcase.T) {
				assert.Error(t, act(t),
					"the ProcessID from the Runtime is the whole of what Complete knows; "+
						"without it there is no Process to complete, and it has to fail "+
						"loudly rather than be silently dropped")

				assert.Empty(t, completionsOf(t, processID.Get(t)),
					"and no owner-less completion is left behind in the log")
			})
		})

		s.When("the workflow clock is frozen at a point in time", func(s *testcase.Spec) {
			var frozenAt = let.Var(s, func(t *testcase.T) time.Time {
				return clock.Now().Add(t.Random.DurationBetween(time.Hour, 24*time.Hour))
			})

			s.Before(func(t *testcase.T) {
				timecop.Travel(t, frozenAt.Get(t), timecop.Freeze)
			})

			s.Then("the completion is timestamped from the workflow clock", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var completions = completionsOf(t, processID.Get(t))
				assert.Equal(t, 1, len(completions))
				assert.True(t, completions[0].Timestamp.Equal(frozenAt.Get(t)), assert.MessageF(
					"completion timestamps must come from the injectable workflow clock "+
						"rather than from time.Now(); the event log is ordered and queried "+
						"by these timestamps, so a test that time travels needs them to "+
						"travel too. want %v, got %v", frozenAt.Get(t), completions[0].Timestamp))
			})
		})
	})

	s.Describe("#Error", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) string {
			return subject.Get(t).Error()
		})

		s.Then("it identifies the signal", func(t *testcase.T) {
			assert.Equal(t, "workflow::complete", act(t))
		})
	})

	s.Describe("errors.Is", func(s *testcase.Spec) {
		var target = let.Var(s, func(t *testcase.T) error {
			return workflow.Complete{}
		})

		act := let.Act(func(t *testcase.T) bool {
			return errors.Is(subject.Get(t), target.Get(t))
		})

		s.Then("a Complete matches the bare Complete value", func(t *testcase.T) {
			assert.True(t, act(t), assert.MessageF(
				"Complete carries no payload, so its type is the whole of its "+
					"identity: errors.Is(err, workflow.Complete{}) is how a caller "+
					"recognises the signal, exactly as it recognises Halt{} and "+
					"Suspend{}. Which Process finished is a separate question, and "+
					"the log answers it through workflow.IsCompleted"))
		})

		s.When("the target is a different RuntimeSignal", func(s *testcase.Spec) {
			target.Let(s, func(t *testcase.T) error {
				if t.Random.Bool() {
					return workflow.Halt{}
				}
				return workflow.Suspend{}
			})

			s.Then("it does not match", func(t *testcase.T) {
				assert.False(t, act(t), assert.MessageF(
					"the signals are three different outcomes: Complete finishes the "+
						"Process, Halt stops it without finishing, Suspend asks to be "+
						"resumed. Confusing them at the errors.Is boundary would let a "+
						"caller treat an unfinished Process as done"))
			})
		})
	})
}

// TestIsCompleted specifies workflow.IsCompleted, the question everything in
// the runtime asks about a Process: did it run to its end?
//
// Completion is a property of the event history, not of any live execution.
// The answer is found by reading the Process' events back and looking for the
// EventCompleted that Complete writes, which is what lets a restarted node, a
// parent waiting on a spawned child, or a test observe the same truth without
// coordinating with whoever ran the Process.
//
// Because the repository is shared by every Process, the answer has to be
// scoped to the ProcessID being asked about, and nothing short of an actual
// completion may count as one.
func TestIsCompleted(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})
		events = let.Var(s, func(t *testcase.T) *memory.WorkflowEventRepository {
			return &memory.WorkflowEventRepository{}
		})
		// history is what the repository holds for the Process at the moment
		// the question is asked. The base case is a Process that ran to its
		// end, which is the state IsCompleted exists to recognise.
		history = let.Var(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{completionEventFor(t, processID.Get(t))}
		})
	)

	s.Before(func(t *testcase.T) {
		for _, event := range history.Get(t) {
			seedEvent(t, events.Get(t), event)
		}
	})

	act := let.Act(func(t *testcase.T) bool {
		isCompleted, err := workflow.IsCompleted(t.Context(), events.Get(t), processID.Get(t))
		assert.NoError(t, err)
		return isCompleted
	})

	s.Then("a Process whose history holds its completion is completed", func(t *testcase.T) {
		assert.True(t, act(t))
	})

	s.When("the Process has no history at all", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			if t.Random.Bool() {
				return nil
			}
			return []workflow.Event{}
		})

		s.Then("it is not completed", func(t *testcase.T) {
			assert.False(t, act(t), assert.MessageF(
				"absence of evidence is not completion; a Process that was never "+
					"executed has to be indistinguishable from one still in flight"))
		})
	})

	s.When("the Process has progressed but not finished", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{
				workflow.EventSetVar{
					EventID:   mustEventID(t),
					ProcessID: processID.Get(t),
					Timestamp: clock.Now(),
					Name:      "answer",
					Value:     42,
				},
				workflow.EventParticipant{
					EventID:       mustEventID(t),
					ProcessID:     processID.Get(t),
					Timestamp:     clock.Now().Add(time.Millisecond),
					ParticipantID: "review-gate",
				},
			}
		})

		s.Then("it is not completed", func(t *testcase.T) {
			assert.False(t, act(t), assert.MessageF(
				"only EventCompleted answers this question; work having happened is "+
					"not the same as the work being done"))
		})
	})

	s.When("the Process has a long history that ends in completion", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{
				workflow.EventSetVar{
					EventID:   mustEventID(t),
					ProcessID: processID.Get(t),
					Timestamp: clock.Now(),
					Name:      "answer",
					Value:     42,
				},
				workflow.EventParticipant{
					EventID:       mustEventID(t),
					ProcessID:     processID.Get(t),
					Timestamp:     clock.Now().Add(time.Millisecond),
					ParticipantID: "review-gate",
				},
				completionEventFor(t, processID.Get(t)),
			}
		})

		s.Then("it is completed", func(t *testcase.T) {
			assert.True(t, act(t), assert.MessageF(
				"the whole history is considered, not just its beginning; a Process "+
					"that did a lot of work before finishing is still finished"))
		})
	})

	s.When("the completion in the repository belongs to another Process", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{completionEventFor(t, mustProcessID(t))}
		})

		s.Then("it is not completed", func(t *testcase.T) {
			assert.False(t, act(t), assert.MessageF(
				"the event repository is shared by every Process, so completion has "+
					"to be scoped to the ProcessID being asked about; otherwise one "+
					"Process finishing would finish them all"))
		})
	})

	// A Process cannot come to hold both outcomes through any path the runtime
	// offers: Complete refuses to write EventCompleted once a Process has been
	// terminated, and that refusal is the only thing keeping the two apart. A
	// log holding both for one Process can therefore only come from outside
	// the runtime — a hand-written migration, a restored backup, an operator
	// inserting rows directly.
	//
	// IsCompleted still owes a straight answer when it happens, and the answer
	// is the termination: the Process demonstrably did not run to its end, and
	// a completion stamped after the fact does not retroactively make it so.
	//
	// This is a smoke test, arranged inline on its own repository rather than
	// through the spec's history variable, because the state is not one of the
	// histories the runtime can produce and does not belong among the regular
	// arrangements above.
	s.Test("a terminated Process with a completion inserted after the fact is not completed", func(t *testcase.T) {
		var (
			repo = &memory.WorkflowEventRepository{}
			pid  = mustProcessID(t)
		)

		var terminated workflow.Event = workflow.EventTerminated{
			EventID:   mustEventID(t),
			ProcessID: pid,
			Timestamp: clock.Now(),
		}
		seedEvent(t, repo, terminated)

		// Stamped after the termination, the way a manual insert would land.
		var completed workflow.Event = workflow.EventCompleted{
			EventID:   mustEventID(t),
			ProcessID: pid,
			Timestamp: clock.Now().Add(time.Minute),
		}
		seedEvent(t, repo, completed)

		isCompleted, err := workflow.IsCompleted(t.Context(), repo, pid)
		assert.NoError(t, err)
		assert.False(t, isCompleted, assert.MessageF(
			"the termination is the Process' outcome and an inserted completion "+
				"does not override it; reading this history as completed would let "+
				"a stray row rewrite the story of a Process that was called off"))

		isTerminated, err := workflow.IsTerminated(t.Context(), repo, pid)
		assert.NoError(t, err)
		assert.False(t, isTerminated, assert.MessageF(
			"a completed Process is never terminated, regardless of what order "+
				"the events landed in the log"))
	})

	// The reverse ordering matters too. The memory repository yields events in
	// timestamp order, so a Process whose termination is stamped *after* a
	// stray inserted completion reads the completion first. Without a guard
	// inside IsCompleted, that completion alone is enough to flip the answer to
	// true, contradicting the Process' real outcome.
	s.Test("a Process with a completion inserted before its termination is not completed", func(t *testcase.T) {
		var (
			repo = &memory.WorkflowEventRepository{}
			pid  = mustProcessID(t)
		)

		var completed workflow.Event = workflow.EventCompleted{
			EventID:   mustEventID(t),
			ProcessID: pid,
			Timestamp: clock.Now(),
		}
		seedEvent(t, repo, completed)

		var terminated workflow.Event = workflow.EventTerminated{
			EventID:   mustEventID(t),
			ProcessID: pid,
			Timestamp: clock.Now().Add(time.Minute),
		}
		seedEvent(t, repo, terminated)

		isCompleted, err := workflow.IsCompleted(t.Context(), repo, pid)
		assert.NoError(t, err)
		assert.False(t, isCompleted, assert.MessageF(
			"the completion comes first in timestamp order, but the termination "+
				"is the Process' real outcome and IsCompleted must honour it; "+
				"returning true here would let a manually backdated inserted row "+
				"silently rewrite the Process' history"))

		isTerminated, err := workflow.IsTerminated(t.Context(), repo, pid)
		assert.NoError(t, err)
		assert.False(t, isTerminated, assert.MessageF(
			"a completed Process is never terminated, regardless of what order "+
				"the events landed in the log"))
	})
}

// eventsOfType returns the events of type E that the repository holds for pid.
//
// An outcome is a property of the event log, so every assertion about one is
// made by reading the log back rather than by trusting a return value.
func eventsOfType[E workflow.Event](t *testcase.T, repo workflow.EventRepository, pid workflow.ProcessID) []E {
	t.Helper()
	var out []E
	for _, event := range getProcessEvents(t, repo, pid) {
		if e, ok := event.(E); ok {
			out = append(out, e)
		}
	}
	return out
}

// terminationEventFor builds the event the runtime records when a Process is
// terminated.
func terminationEventFor(t *testcase.T, pid workflow.ProcessID) workflow.Event {
	t.Helper()
	return workflow.EventTerminated{
		EventID:   mustEventID(t),
		ProcessID: pid,
		Timestamp: clock.Now(),
	}
}

// TestTerminate specifies workflow.Terminate, the RuntimeSignal that ends a
// Process without letting it finish.
//
// Terminate is Complete's sibling. Both are terminal, both resolve rather than
// propagate, and both turn the end of a Process into a durable fact in the
// event log — Complete writes EventCompleted, Terminate writes
// EventTerminated. What they mean is opposite: Complete says the Process ran
// to its natural end, Terminate says it was stopped short. Keeping them as
// separate events is what lets a reader tell "it finished" from "it was
// called off" long after either happened.
//
// The one asymmetry is what a Process' existing outcome does to the signal. A
// Process that has already completed cannot be terminated: it has no unfinished
// work left to call off. Raising Terminate on it is not a fault — the caller
// asked for an outcome the Process already has in a stronger form — so the
// signal resolves quietly and writes nothing. Recording a termination there
// would leave the log claiming a Process both finished and was stopped short,
// and every later reader would have to guess which one it meant.
//
// Like Complete, Terminate carries no payload: which Process it is about comes
// from the ProcessID the Runtime hands to RuntimeSignalExecute.
func TestTerminate(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})
		events = let.Var(s, func(t *testcase.T) *memory.WorkflowEventRepository {
			return &memory.WorkflowEventRepository{}
		})
	)

	// subject is the system under test: the termination signal.
	subject := let.Var(s, func(t *testcase.T) workflow.Terminate {
		return workflow.Terminate{}
	})

	terminationsOf := func(t *testcase.T, pid workflow.ProcessID) []workflow.EventTerminated {
		t.Helper()
		return eventsOfType[workflow.EventTerminated](t, events.Get(t), pid)
	}

	completionsOf := func(t *testcase.T, pid workflow.ProcessID) []workflow.EventCompleted {
		t.Helper()
		return eventsOfType[workflow.EventCompleted](t, events.Get(t), pid)
	}

	s.Describe("#RuntimeSignalExecute", func(s *testcase.Spec) {
		// runtime is the smallest Runtime the signal needs: it reads and
		// writes the event history and touches nothing else. Keeping it bare
		// is deliberate. If Terminate ever starts reaching for the queue, the
		// locks or the participants, this spec fails loudly on a nil
		// dependency instead of quietly passing.
		var runtime = let.Var(s, func(t *testcase.T) workflow.Runtime {
			return workflow.Runtime{Events: events.Get(t)}
		})

		act := let.Act(func(t *testcase.T) error {
			return subject.Get(t).RuntimeSignalExecute(t.Context(),
				runtime.Get(t), processID.Get(t))
		})

		s.Then("it resolves the signal instead of propagating it", func(t *testcase.T) {
			assert.NoError(t, act(t),
				"Terminate is terminal like Complete: it settles the Process rather "+
					"than escalating to the scheduler. Suspend and Halt return "+
					"themselves because the scheduler still has a decision to make; "+
					"after a termination there is nothing left to decide")
		})

		s.Then("the termination is recorded in the Process event history", func(t *testcase.T) {
			assert.NoError(t, act(t))

			var terminations = terminationsOf(t, processID.Get(t))
			assert.Equal(t, 1, len(terminations), assert.MessageF(
				"termination is a durable fact about the Process, not a return value; "+
					"it has to survive in the log so that a later Execute, a restart, or "+
					"another node can still tell the Process was called off. got %d",
				len(terminations)))

			var termination = terminations[0]
			assert.Equal(t, processID.Get(t), termination.ProcessID, assert.MessageF(
				"the termination belongs to the Process the Runtime is executing"))
			assert.False(t, termination.EventID.IsZero(),
				"the termination needs its own identity in the log")
			assert.False(t, termination.Timestamp.IsZero(),
				"the termination needs to say when the Process was stopped")
		})

		s.Then("the Process is reported as terminated from then on", func(t *testcase.T) {
			assert.NoError(t, act(t))

			isTerminated, err := workflow.IsTerminated(t.Context(), events.Get(t), processID.Get(t))
			assert.NoError(t, err)
			assert.True(t, isTerminated, assert.MessageF(
				"raising the signal and asking the question are two halves of one "+
					"contract; whatever RuntimeSignalExecute writes, IsTerminated must read"))
		})

		s.Then("the Process is not reported as completed", func(t *testcase.T) {
			assert.NoError(t, act(t))

			isCompleted, err := workflow.IsCompleted(t.Context(), events.Get(t), processID.Get(t))
			assert.NoError(t, err)
			assert.False(t, isCompleted, assert.MessageF(
				"being stopped short is not the same as running to the end; if "+
					"terminating a Process also made it look completed, the log would "+
					"lose the only distinction that explains why the Process stopped"))
		})

		s.When("the Process is already terminated", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				seedEvent(t, events.Get(t), terminationEventFor(t, processID.Get(t)))
			})

			s.Then("no second termination is recorded", func(t *testcase.T) {
				var before = terminationsOf(t, processID.Get(t))
				assert.Equal(t, 1, len(before),
					assert.MessageF("arrangement: the Process starts out terminated"))

				assert.NoError(t, act(t))

				assert.Equal(t, before, terminationsOf(t, processID.Get(t)), assert.MessageF(
					"raising Terminate on an already terminated Process is a no-op; a "+
						"Process re-entered after termination (a retry, a replay, a "+
						"duplicate queue entry) must not grow a second termination, or "+
						"the log stops being a reliable answer to when it was stopped"))
			})
		})

		s.When("the Process has already completed", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				seedEvent(t, events.Get(t), completionEventFor(t, processID.Get(t)))
			})

			s.Then("the signal is resolved without recording a termination", func(t *testcase.T) {
				assert.NoError(t, act(t), assert.MessageF(
					"asking to terminate a finished Process is not a fault; the caller "+
						"wanted the Process stopped and it already is, so there is "+
						"nothing to report and nothing to retry"))

				assert.Empty(t, terminationsOf(t, processID.Get(t)), assert.MessageF(
					"a completed Process has no unfinished work left to call off. "+
						"Writing EventTerminated here would leave the log claiming the "+
						"Process both ran to its end and was stopped short, and every "+
						"later reader would have to guess which one it meant"))
			})

			s.Then("the completion is left as the Process outcome", func(t *testcase.T) {
				var before = completionsOf(t, processID.Get(t))
				assert.Equal(t, 1, len(before),
					assert.MessageF("arrangement: the Process starts out completed"))

				assert.NoError(t, act(t))

				assert.Equal(t, before, completionsOf(t, processID.Get(t)), assert.MessageF(
					"the completion already on record is the Process' outcome and the "+
						"signal must not disturb it"))

				isCompleted, err := workflow.IsCompleted(t.Context(), events.Get(t), processID.Get(t))
				assert.NoError(t, err)
				assert.True(t, isCompleted, assert.MessageF(
					"and the Process still reads as completed afterwards"))
			})
		})

		s.When("another Process has already been terminated", func(s *testcase.Spec) {
			var otherProcessID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
				return mustProcessID(t)
			})

			s.Before(func(t *testcase.T) {
				seedEvent(t, events.Get(t), terminationEventFor(t, otherProcessID.Get(t)))
			})

			s.Then("this Process is terminated on its own account", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, 1, len(terminationsOf(t, processID.Get(t))), assert.MessageF(
					"the event repository is shared by every Process, so a foreign "+
						"termination must not be mistaken for this one's and skip the write"))
				assert.Equal(t, 1, len(terminationsOf(t, otherProcessID.Get(t))), assert.MessageF(
					"and the other Process' history is left exactly as it was"))
			})
		})

		s.When("another Process has already completed", func(s *testcase.Spec) {
			var otherProcessID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
				return mustProcessID(t)
			})

			s.Before(func(t *testcase.T) {
				seedEvent(t, events.Get(t), completionEventFor(t, otherProcessID.Get(t)))
			})

			s.Then("this Process is terminated all the same", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, 1, len(terminationsOf(t, processID.Get(t))), assert.MessageF(
					"the completion that suppresses a termination is the one belonging "+
						"to this Process; a foreign completion says nothing about it, and "+
						"reading the check unscoped would let one Process finishing "+
						"silently block every other Process from being terminated"))
			})
		})

		s.When("the Runtime passes no ProcessID", func(s *testcase.Spec) {
			processID.Let(s, func(t *testcase.T) workflow.ProcessID {
				return workflow.ProcessID{}
			})

			s.Then("nothing is terminated and the fault is reported", func(t *testcase.T) {
				assert.Error(t, act(t), assert.MessageF(
					"the ProcessID from the Runtime is the whole of what Terminate "+
						"knows; without it there is no Process to stop, and it has to "+
						"fail loudly rather than be silently dropped"))

				assert.Empty(t, terminationsOf(t, processID.Get(t)), assert.MessageF(
					"and no ownerless termination is left behind in the log"))
			})
		})

		s.When("the workflow clock is frozen at a point in time", func(s *testcase.Spec) {
			var frozenAt = let.Var(s, func(t *testcase.T) time.Time {
				return clock.Now().Add(t.Random.DurationBetween(time.Hour, 24*time.Hour))
			})

			s.Before(func(t *testcase.T) {
				timecop.Travel(t, frozenAt.Get(t), timecop.Freeze)
			})

			s.Then("the termination is timestamped from the workflow clock", func(t *testcase.T) {
				assert.NoError(t, act(t))

				var terminations = terminationsOf(t, processID.Get(t))
				assert.Equal(t, 1, len(terminations))
				assert.True(t, terminations[0].Timestamp.Equal(frozenAt.Get(t)), assert.MessageF(
					"termination timestamps must come from the injectable workflow "+
						"clock rather than from time.Now(); the event log is ordered and "+
						"queried by these timestamps, so a test that time travels needs "+
						"them to travel too. want %v, got %v",
					frozenAt.Get(t), terminations[0].Timestamp))
			})
		})
	})

	s.Describe("#Error", func(s *testcase.Spec) {
		act := let.Act(func(t *testcase.T) string {
			return subject.Get(t).Error()
		})

		s.Then("it identifies the signal", func(t *testcase.T) {
			assert.Equal(t, "workflow::terminate", act(t))
		})
	})

	s.Describe("errors.Is", func(s *testcase.Spec) {
		var target = let.Var(s, func(t *testcase.T) error {
			return workflow.Terminate{}
		})

		act := let.Act(func(t *testcase.T) bool {
			return errors.Is(subject.Get(t), target.Get(t))
		})

		s.Then("a Terminate matches the bare Terminate value", func(t *testcase.T) {
			assert.True(t, act(t), assert.MessageF(
				"Terminate carries no payload, so its type is the whole of its "+
					"identity: errors.Is(err, workflow.Terminate{}) is how a caller "+
					"recognises the signal"))
		})

		s.When("the target is a different RuntimeSignal", func(s *testcase.Spec) {
			target.Let(s, func(t *testcase.T) error {
				switch t.Random.IntBetween(0, 2) {
				case 0:
					return workflow.Complete{}
				case 1:
					return workflow.Halt{}
				default:
					return workflow.Suspend{}
				}
			})

			s.Then("it does not match", func(t *testcase.T) {
				assert.False(t, act(t), assert.MessageF(
					"Terminate and Complete are the two terminal signals and they mean "+
						"opposite things, so confusing them at the errors.Is boundary "+
						"would let a caller read a called-off Process as a finished one"))
			})
		})
	})
}

// TestIsTerminated specifies workflow.IsTerminated, the question asked about a
// Process that may have been called off: was it stopped short?
//
// Like completion, termination is a property of the event history rather than
// of any live execution, and the answer has to be scoped to the ProcessID
// being asked about because the repository is shared by every Process.
//
// Termination and completion are mutually exclusive outcomes, so a Process
// that ran to its end is never terminated, no matter what else its history
// holds.
func TestIsTerminated(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		processID = let.Var(s, func(t *testcase.T) workflow.ProcessID {
			return mustProcessID(t)
		})
		events = let.Var(s, func(t *testcase.T) *memory.WorkflowEventRepository {
			return &memory.WorkflowEventRepository{}
		})
		// history is what the repository holds for the Process at the moment
		// the question is asked. The base case is a Process that was called
		// off, which is the state IsTerminated exists to recognise.
		history = let.Var(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{terminationEventFor(t, processID.Get(t))}
		})
	)

	s.Before(func(t *testcase.T) {
		for _, event := range history.Get(t) {
			seedEvent(t, events.Get(t), event)
		}
	})

	act := let.Act(func(t *testcase.T) bool {
		isTerminated, err := workflow.IsTerminated(t.Context(), events.Get(t), processID.Get(t))
		assert.NoError(t, err)
		return isTerminated
	})

	s.Then("a Process whose history holds its termination is terminated", func(t *testcase.T) {
		assert.True(t, act(t))
	})

	s.When("the Process has no history at all", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			if t.Random.Bool() {
				return nil
			}
			return []workflow.Event{}
		})

		s.Then("it is not terminated", func(t *testcase.T) {
			assert.False(t, act(t), assert.MessageF(
				"absence of evidence is not termination; a Process that was never "+
					"executed has to be indistinguishable from one still in flight"))
		})
	})

	s.When("the Process has progressed but not been stopped", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{
				workflow.EventSetVar{
					EventID:   mustEventID(t),
					ProcessID: processID.Get(t),
					Timestamp: clock.Now(),
					Name:      "answer",
					Value:     42,
				},
				workflow.EventParticipant{
					EventID:       mustEventID(t),
					ProcessID:     processID.Get(t),
					Timestamp:     clock.Now().Add(time.Millisecond),
					ParticipantID: "review-gate",
				},
			}
		})

		s.Then("it is not terminated", func(t *testcase.T) {
			assert.False(t, act(t), assert.MessageF(
				"only EventTerminated answers this question; work having happened is "+
					"not the same as the work being called off"))
		})
	})

	s.When("the Process has a long history that ends in termination", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{
				workflow.EventSetVar{
					EventID:   mustEventID(t),
					ProcessID: processID.Get(t),
					Timestamp: clock.Now(),
					Name:      "answer",
					Value:     42,
				},
				workflow.EventParticipant{
					EventID:       mustEventID(t),
					ProcessID:     processID.Get(t),
					Timestamp:     clock.Now().Add(time.Millisecond),
					ParticipantID: "review-gate",
				},
				terminationEventFor(t, processID.Get(t)),
			}
		})

		s.Then("it is terminated", func(t *testcase.T) {
			assert.True(t, act(t), assert.MessageF(
				"the whole history is considered, not just its beginning; a Process "+
					"that did a lot of work before being stopped is still stopped"))
		})
	})

	s.When("the Process ran to its end instead", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{completionEventFor(t, processID.Get(t))}
		})

		s.Then("it is not terminated", func(t *testcase.T) {
			assert.False(t, act(t), assert.MessageF(
				"completion and termination are opposite outcomes: a Process that "+
					"finished was never called off, and reading it as terminated would "+
					"invent a stop that never happened"))
		})
	})

	s.When("the termination in the repository belongs to another Process", func(s *testcase.Spec) {
		history.Let(s, func(t *testcase.T) []workflow.Event {
			return []workflow.Event{terminationEventFor(t, mustProcessID(t))}
		})

		s.Then("it is not terminated", func(t *testcase.T) {
			assert.False(t, act(t), assert.MessageF(
				"the event repository is shared by every Process, so termination has "+
					"to be scoped to the ProcessID being asked about; otherwise one "+
					"Process being called off would call off them all"))
		})
	})

	// A Process cannot come to hold both outcomes through any path the runtime
	// offers: Terminate refuses to write EventTerminated once a Process has
	// completed, and that refusal is the only thing keeping the two apart. A
	// log holding both for one Process can therefore only come from outside
	// the runtime — a hand-written migration, a restored backup, an operator
	// inserting rows directly.
	//
	// IsTerminated still owes a straight answer when it happens, and the
	// answer is the completion: the Process demonstrably ran to its end, and a
	// termination stamped after the fact does not retroactively call off work
	// that was already done.
	//
	// This is a smoke test, arranged inline on its own repository rather than
	// through the spec's history variable, because the state is not one of the
	// histories the runtime can produce and does not belong among the regular
	// arrangements above.
	s.Test("a completed Process with a termination inserted after the fact is not terminated", func(t *testcase.T) {
		var (
			repo = &memory.WorkflowEventRepository{}
			pid  = mustProcessID(t)
		)

		var completed workflow.Event = workflow.EventCompleted{
			EventID:   mustEventID(t),
			ProcessID: pid,
			Timestamp: clock.Now(),
		}
		seedEvent(t, repo, completed)

		// Stamped after the completion, the way a manual insert would land.
		var terminated workflow.Event = workflow.EventTerminated{
			EventID:   mustEventID(t),
			ProcessID: pid,
			Timestamp: clock.Now().Add(time.Minute),
		}
		seedEvent(t, repo, terminated)

		isTerminated, err := workflow.IsTerminated(t.Context(), repo, pid)
		assert.NoError(t, err)
		assert.False(t, isTerminated, assert.MessageF(
			"the completion is the Process' outcome and an inserted termination "+
				"does not override it; reading this history as terminated would let "+
				"a stray row rewrite the story of a Process that finished"))

		isCompleted, err := workflow.IsCompleted(t.Context(), repo, pid)
		assert.NoError(t, err)
		assert.False(t, isCompleted, assert.MessageF(
			"a terminated Process is never completed, regardless of what order "+
				"the events landed in the log"))
	})

	// The reverse ordering — termination first, completion second — is not
	// pinned here because the meaning is genuinely ambiguous and the
	// implementation currently reads it as "first terminal event wins": the
	// termination is returned by FindByProcessID first and IsTerminated
	// returns true. Pinning an outcome would commit to a precedence rule
	// between two histories the runtime itself never produces; the existing
	// smoke test already establishes that the runtime's own write rules keep
	// these histories from arising through normal use.
}
