package memory_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/crud/crudcontract"

	"go.llib.dev/testcase/assert"
)

func TestWorkflowEventRepository(t *testing.T) {
	ctx := context.Background()

	pidA, err := workflow.MakeProcessID()
	assert.NoError(t, err)
	pidB, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	mkEvent := func(pid workflow.ProcessID, key workflow.VariableKey, val any) workflow.Event {
		var e workflow.Event = &workflow.VariableEvent{
			Operation: workflow.SetVariableEventOperation,
			Key:       key,
			Value:     val,
		}
		e.SetProcessID(pid)
		return e
	}

	t.Run("FindByProcessID returns only the matching process events, in insertion order", func(t *testing.T) {
		repo := &memory.WorkflowEventRepository{}

		// events of the two processes are interleaved
		a1 := mkEvent(pidA, "a", 1)
		b1 := mkEvent(pidB, "b", 1)
		a2 := mkEvent(pidA, "a", 2)

		assert.NoError(t, repo.Create(ctx, &a1))
		assert.NoError(t, repo.Create(ctx, &b1))
		assert.NoError(t, repo.Create(ctx, &a2))

		gotA, err := iterkit.CollectE(repo.FindByProcessID(ctx, pidA))
		assert.NoError(t, err)
		assert.Equal(t, []workflow.Event{a1, a2}, gotA)

		gotB, err := iterkit.CollectE(repo.FindByProcessID(ctx, pidB))
		assert.NoError(t, err)
		assert.Equal(t, []workflow.Event{b1}, gotB)
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
		base := mkEvent(pidA, "base", 0)
		assert.NoError(t, repo.Create(ctx, &base))

		txCtx, err := repo.BeginTx(ctx)
		assert.NoError(t, err)
		inTx := mkEvent(pidA, "in-tx", 1)
		assert.NoError(t, repo.Create(txCtx, &inTx))
		assert.NoError(t, repo.RollbackTx(txCtx))

		got, err := iterkit.CollectE(repo.FindByProcessID(ctx, pidA))
		assert.NoError(t, err)
		assert.Equal(t, []workflow.Event{base}, got)
	})

	t.Run("commit keeps the events created within the transaction", func(t *testing.T) {
		repo := &memory.WorkflowEventRepository{}
		base := mkEvent(pidA, "base", 0)
		assert.NoError(t, repo.Create(ctx, &base))

		txCtx, err := repo.BeginTx(ctx)
		assert.NoError(t, err)
		inTx := mkEvent(pidA, "in-tx", 1)
		assert.NoError(t, repo.Create(txCtx, &inTx))
		assert.NoError(t, repo.CommitTx(txCtx))

		got, err := iterkit.CollectE(repo.FindByProcessID(ctx, pidA))
		assert.NoError(t, err)
		assert.Equal(t, []workflow.Event{base, inTx}, got)
	})

	conf := crudcontract.Config[workflow.Event, string]{}

	crudcontract.Creator()
}
