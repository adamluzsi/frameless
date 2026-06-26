package memory

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
)

// WorkflowEventRepository is an in-memory workflow.EventsRepository.
//
// It records workflow events in an append-only fashion, preserving their
// creation order. The workflow engine relies on this ordering while replaying
// the event history (idempotency checks, variable resolution, completion
// detection). Events are associated with a Process through Event.ProcessID, so
// FindByProcessID can return the history of a single Process.
//
// It is primarily intended for tests and local development.
type WorkflowEventRepository[EventID ~string | ~uuid.UUID] struct {
	mu     sync.RWMutex
	events []workflow.Event
}

var (
	_ workflow.EventsRepository       = (*WorkflowEventRepository)(nil)
	_ comproto.OnePhaseCommitProtocol = (*WorkflowEventRepository)(nil)
	_ crud.Creator[workflow.Event]    = (*WorkflowEventRepository)(nil)
)

func (r *WorkflowEventRepository) Create(ctx context.Context, ptr *workflow.Event) error {
	if ptr == nil || *ptr == nil {
		return fmt.Errorf("memory: nil workflow.Event passed to WorkflowEventRepository.Create")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx, ok := r.lookupTx(ctx); ok && tx.done {
		return errTxDone
	}
	r.mu.Lock()
	r.events = append(r.events, *ptr)
	r.mu.Unlock()
	return nil
}

func (r *WorkflowEventRepository) FindByProcessID(ctx context.Context, pid workflow.ProcessID) iter.Seq2[workflow.Event, error] {
	return func(yield func(workflow.Event, error) bool) {
		if err := ctx.Err(); err != nil {
			var zero workflow.Event
			yield(zero, err)
			return
		}
		for _, e := range r.snapshot() {
			if e == nil {
				continue
			}
			if e.ProcessID() != pid {
				continue
			}
			if !yield(e, nil) {
				return
			}
		}
	}
}

func (r *WorkflowEventRepository) snapshot() []workflow.Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slicekit.Clone(r.events)
}

type ctxKeyWorkflowEventRepoTx struct{ repo *WorkflowEventRepository }

type workflowEventRepoTx struct {
	snapshot []workflow.Event
	done     bool
}

func (r *WorkflowEventRepository) BeginTx(ctx context.Context) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return ctx, err
	}
	r.mu.RLock()
	snapshot := slicekit.Clone(r.events)
	r.mu.RUnlock()
	return context.WithValue(ctx, ctxKeyWorkflowEventRepoTx{r}, &workflowEventRepoTx{snapshot: snapshot}), nil
}

func (r *WorkflowEventRepository) CommitTx(ctx context.Context) error {
	tx, ok := r.lookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.done {
		return errTxDone
	}
	tx.done = true
	return nil
}

func (r *WorkflowEventRepository) RollbackTx(ctx context.Context) error {
	tx, ok := r.lookupTx(ctx)
	if !ok {
		return errNoTx
	}
	if tx.done {
		return errTxDone
	}
	tx.done = true
	r.mu.Lock()
	r.events = tx.snapshot
	r.mu.Unlock()
	return nil
}

func (r *WorkflowEventRepository) lookupTx(ctx context.Context) (*workflowEventRepoTx, bool) {
	if ctx == nil {
		return nil, false
	}
	tx, ok := ctx.Value(ctxKeyWorkflowEventRepoTx{r}).(*workflowEventRepoTx)
	return tx, ok
}
