package memory

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/uuid"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/pubsub"
)

// WorkflowEventRepository is an in-memory workflow.EventRepository.
//
// It records workflow events in an append-only fashion, preserving their
// creation order. The workflow engine relies on this ordering while replaying
// the event history (idempotency checks, variable resolution, completion
// detection). Events are associated with a Process through Event.ProcessID, so
// FindByProcessID can return the history of a single Process.
//
// It is primarily intended for tests and local development.
type WorkflowEventRepository struct {
	r Repository[workflow.Event, workflow.EventID]

	o sync.Once
	s atomic.Uint64
}

var _ workflow.EventRepository = (*WorkflowEventRepository)(nil)

func (r *WorkflowEventRepository) init() {
	r.o.Do(func() {
		r.r = Repository[workflow.Event, workflow.EventID]{
			Memory: NewMemory(),
			MakeID: func(ctx context.Context) (workflow.EventID, error) {
				return uuid.MakeV7()
			},
		}
	})
}

var _ comproto.OnePhaseCommitProtocol = (*WorkflowEventRepository)(nil)

func (r *WorkflowEventRepository) LookupTx(ctx context.Context) (*MemoryTx, bool) {
	r.init()
	return r.r.Memory.LookupTx(ctx)
}

func (r *WorkflowEventRepository) BeginTx(ctx context.Context) (context.Context, error) {
	r.init()
	return r.r.BeginTx(ctx)
}

func (r *WorkflowEventRepository) CommitTx(ctx context.Context) error {
	r.init()
	return r.r.CommitTx(ctx)
}

func (r *WorkflowEventRepository) RollbackTx(ctx context.Context) error {
	r.init()
	return r.r.RollbackTx(ctx)
}

var _ crud.Creator[workflow.Event] = (*WorkflowEventRepository)(nil)

func (r *WorkflowEventRepository) Create(ctx context.Context, ptr *workflow.Event) error {
	r.init()
	if err := r.validateEvent(ctx, ptr); err != nil {
		return err
	}
	return r.r.Create(ctx, ptr)
}

var _ crud.Updater[workflow.Event] = (*WorkflowEventRepository)(nil)

func (r *WorkflowEventRepository) Update(ctx context.Context, ptr *workflow.Event) error {
	r.init()
	if err := r.validateEvent(ctx, ptr); err != nil {
		return err
	}
	return r.r.Update(ctx, ptr)
}

var _ crud.ByIDFinder[workflow.Event, workflow.EventID] = (*WorkflowEventRepository)(nil)

func (r *WorkflowEventRepository) FindByID(ctx context.Context, id workflow.EventID) (workflow.Event, bool, error) {
	r.init()
	return r.r.FindByID(ctx, id)
}

var _ crud.ByIDDeleter[workflow.EventID] = (*WorkflowEventRepository)(nil)

func (r *WorkflowEventRepository) DeleteByID(ctx context.Context, id workflow.EventID) error {
	r.init()
	return r.r.DeleteByID(ctx, id)
}

var _ crud.AllFinder[workflow.Event] = (*WorkflowEventRepository)(nil)

func (r *WorkflowEventRepository) FindAll(ctx context.Context) iter.Seq2[workflow.Event, error] {
	r.init()
	return r.r.FindAll(ctx)
}

func (r *WorkflowEventRepository) FindByProcessID(ctx context.Context, pid workflow.ProcessID) iter.Seq2[workflow.Event, error] {
	return func(yield func(workflow.Event, error) bool) {
		events, err := iterkit.CollectE(iterkit.Filter(r.FindAll(ctx), func(e workflow.Event) bool {
			return e.GetProcessID().Equal(pid)
		}))

		if err != nil {
			yield(nil, err)
			return
		}

		slicekit.SortBy(events, func(a, b workflow.Event) bool {
			return a.GetTimestamp().Before(b.GetTimestamp())
		})

		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func (r *WorkflowEventRepository) validateEvent(ctx context.Context, ptr *workflow.Event) error {
	if ptr == nil || *ptr == nil {
		return errorkit.F("nil %T received", ptr)
	}
	if (*ptr).GetEventID().IsZero() {
		return errorkit.F("zero workflow event id")
	}
	if (*ptr).GetProcessID().IsZero() {
		return errorkit.F("zero workflow event process id")
	}
	if (*ptr).GetTimestamp().IsZero() {
		return errorkit.F("zero workflow event timestamp")
	}
	return nil
}

type WorkflowProcessChangeBroadcast struct {
	exchange FanOutExchange[workflow.ProcessChangeEvent]
}

var _ workflow.ProcessChangeBroadcast = (*WorkflowProcessChangeBroadcast)(nil)

func (ex *WorkflowProcessChangeBroadcast) Publish(ctx context.Context, event workflow.ProcessChangeEvent) error {
	return ex.exchange.Publish(ctx, event)
}

func (ex *WorkflowProcessChangeBroadcast) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ProcessChangeEvent] {
	q := ex.exchange.MakeQueue()
	q.Volatile = true
	return q.Subscribe(ctx)
}

type WorkflowProcessExecutionQueue struct {
	q *Queue[workflow.ProcessExecution]
	o sync.Once
}

var _ workflow.ProcessExecutionQueue = (*WorkflowProcessExecutionQueue)(nil)

func (q *WorkflowProcessExecutionQueue) init() {
	q.o.Do(func() {
		q.q = &Queue[workflow.ProcessExecution]{

			SortLessFunc: func(i, j workflow.ProcessExecution) bool {
				return i.StartTime.Before(j.StartTime)
			},
		}
	})
}

func (q *WorkflowProcessExecutionQueue) Publish(ctx context.Context, pe workflow.ProcessExecution) error {
	q.init()
	return q.q.Publish(ctx, pe)
}

func (q *WorkflowProcessExecutionQueue) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ProcessExecution] {
	q.init()
	return q.q.Subscribe(ctx)
}
