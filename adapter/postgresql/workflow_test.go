package postgresql_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfcontract"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
)

func TestWorkflowLockerFactory(t *testing.T) {
	c := GetConnection(t)

	var wlf workflow.ProcessLockers = postgresql.WorkflowLockerFactory{Connection: c}

	subject := &memory.LockerFactory[workflow.ProcessID, guard.NonBlockingLocker]{}
	t.Run("implements workflow ProcessLockers", wfcontract.ProcessLockers(subject).Test)
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
