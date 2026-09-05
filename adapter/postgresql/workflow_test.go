package postgresql_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfcontract"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
)

func Test_workflowE2E(t *testing.T) {
	ctx := context.Background()
	c := GetConnection(t)

	events := &postgresql.WorkflowEventRepository{Connection: c}
	assert.NoError(t, events.Migrate(ctx))

	queue := &postgresql.WorkflowQueue{Connection: c}
	assert.NoError(t, queue.Migrate(ctx))
	t.Cleanup(func() {
		_, _ = c.ExecContext(ctx, `DELETE FROM frameless_queue_messages WHERE queue = 'frameless_workflow_process_executions'`)
	})

	changes := &postgresql.WorkflowProcessChangeBroadcast{}

	rt := workflow.Runtime{
		Events:                 events,
		ProcessExecutionQueue:  queue,
		ProcessChangeBroadcast: changes,
		ProcessLockers:         postgresql.WorkflowLockerFactory{Connection: c},
		NumQueueSubscriber:     1,
		Participants: workflow.Participants{
			"greet": func(ctx context.Context, name string) (string, error) {
				return "Hello, " + name + "!", nil
			},
		},
	}

	pid, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	def := workflow.Sequence{
		workflow.SetVar{Key: "name", Value: "World"},
		workflow.ExecuteParticipant{
			ID:     "greet",
			Input:  []workflow.VarKey{"name"},
			Output: []workflow.VarKey{"greeting"},
		},
	}

	assert.NoError(t, rt.Bind(ctx, pid, def))
	assert.NoError(t, rt.Execute(ctx, pid))

	completed, err := workflow.Complete{}.IsCompleted(ctx, events)
	assert.NoError(t, err)
	assert.True(t, completed, assert.MessageF(
		"runtime should persist EventCompleted for the process after a successful execution"))

	got, err := iterkit.CollectE(events.FindByProcessID(ctx, pid))
	assert.NoError(t, err)
	assert.NotEmpty(t, got)

	var sawUseDefinition, sawParticipant, sawCompleted bool
	for _, event := range got {
		switch event.(type) {
		case workflow.EventUseDefinition:
			sawUseDefinition = true
		case workflow.EventParticipant:
			sawParticipant = true
		case workflow.EventCompleted:
			sawCompleted = true
		}
	}
	assert.True(t, sawUseDefinition, assert.MessageF("Bind should persist EventUseDefinition"))
	assert.True(t, sawParticipant, assert.MessageF("Execute should persist EventParticipant from participant step"))
	assert.True(t, sawCompleted, assert.MessageF("Execute should persist EventCompleted on natural completion"))

	vmap, err := workflow.ProcessVars{ProcessID: pid, EventsRepository: events}.ToMap(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "World", vmap["name"])
	assert.Equal(t, "Hello, World!", vmap["greeting"])
}

func TestWorkflowLockerFactory(t *testing.T) {
	c := GetConnection(t)
	subject := postgresql.WorkflowLockerFactory{Connection: c}
	assert.NoError(t, subject.Migrate(t.Context()))
	wfcontract.ProcessLockers(subject).Test(t)
}

func TestWorkflowProcessExecutionQueue(t *testing.T) {
	var subject memory.WorkflowProcessExecutionQueue
	wfcontract.ProcessExecutionQueue(&subject).Test(t)
}

func TestWorkflowProcessChangeBroadcast(t *testing.T) {
	var subject memory.WorkflowProcessChangeBroadcast
	wfcontract.ProcessChangeBroadcast(&subject).Test(t)
}

func TestWorkflowEventRepository(t *testing.T) {
	var eventsRepo memory.WorkflowEventRepository
	wfcontract.EventRepository(&eventsRepo).Test(t)

	t.Run("smoke", func(t *testing.T) {
		ctx := context.Background()

		pidA, err := workflow.MakeProcessID()
		assert.NoError(t, err)
		pidB, err := workflow.MakeProcessID()
		assert.NoError(t, err)

		var mkEvent = func(tb testing.TB, pid workflow.ProcessID, key workflow.VarKey, val any) workflow.Event {
			eventID, err := workflow.MakeEventID()
			assert.NoError(tb, err)
			var e workflow.Event = workflow.EventVar{
				EventID:   eventID,
				ProcessID: pid,
				Timestamp: clock.Now(),
				Operation: workflow.SetEventVarOperation,
				Key:       key,
				Value:     val,
			}
			return e
		}

		t.Run("FindByProcessID returns only the matching process events, in insertion order", func(t *testing.T) {
			repo := &memory.WorkflowEventRepository{}

			// events of the two processes are interleaved
			a1 := mkEvent(t, pidA, "a", 1)
			b1 := mkEvent(t, pidB, "b", 1)
			a2 := mkEvent(t, pidA, "a", 2)

			assert.NoError(t, repo.Create(ctx, &a1))
			assert.NoError(t, repo.Create(ctx, &b1))
			assert.NoError(t, repo.Create(ctx, &a2))

			all, err := iterkit.CollectE(repo.FindAll(t.Context()))
			assert.NoError(t, err)
			assert.NotEmpty(t, all)
			assert.Equal(t, 3, len(all))
			assert.Contains(t, all, a1)
			assert.Contains(t, all, b1)
			assert.Contains(t, all, a2)

			gotA, err := iterkit.CollectE(repo.FindByProcessID(ctx, pidA))
			assert.NoError(t, err)
			assert.ContainsExactly(t, []workflow.Event{a1, a2}, gotA)

			gotB, err := iterkit.CollectE(repo.FindByProcessID(ctx, pidB))
			assert.NoError(t, err)
			assert.ContainsExactly(t, []workflow.Event{b1}, gotB)
		})

		t.Run("an unknown process id yields no events", func(t *testing.T) {
			repo := &memory.WorkflowEventRepository{}
			unknown, err := workflow.MakeProcessID()
			assert.NoError(t, err)
			got, err := iterkit.CollectE(repo.FindByProcessID(ctx, unknown))
			assert.NoError(t, err)
			assert.Empty(t, got)
		})

		t.Run("a nil event is rejected", func(t *testing.T) {
			repo := &memory.WorkflowEventRepository{}
			var nilEvent workflow.Event
			assert.Error(t, repo.Create(ctx, &nilEvent))
			assert.Error(t, repo.Create(ctx, nil))
		})

		t.Run("rollback discards the events created within the transaction", func(t *testing.T) {
			repo := &memory.WorkflowEventRepository{}
			base := mkEvent(t, pidA, "base", 0)
			assert.NoError(t, repo.Create(ctx, &base))

			txCtx, err := repo.BeginTx(ctx)
			assert.NoError(t, err)
			inTx := mkEvent(t, pidA, "in-tx", 1)
			assert.NoError(t, repo.Create(txCtx, &inTx))
			assert.NoError(t, repo.RollbackTx(txCtx))

			got, err := iterkit.CollectE(repo.FindByProcessID(ctx, pidA))
			assert.NoError(t, err)
			assert.ContainsExactly(t, []workflow.Event{base}, got)
		})

		t.Run("commit keeps the events created within the transaction", func(t *testing.T) {
			repo := &memory.WorkflowEventRepository{}
			base := mkEvent(t, pidA, "base", 0)
			assert.NoError(t, repo.Create(ctx, &base))

			txCtx, err := repo.BeginTx(ctx)
			assert.NoError(t, err)
			inTx := mkEvent(t, pidA, "in-tx", 1)
			assert.NoError(t, repo.Create(txCtx, &inTx))
			assert.NoError(t, repo.CommitTx(txCtx))

			got, err := iterkit.CollectE(repo.FindByProcessID(ctx, pidA))
			assert.NoError(t, err)
			assert.ContainsExactly(t, []workflow.Event{base, inTx}, got)
		})
	})
}
