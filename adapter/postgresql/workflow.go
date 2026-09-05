package postgresql

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"

	"github.com/jackc/pgx/v5"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/pubsub"
)

const (
	workflowEventTableName        = "frameless_workflow_events"
	workflowProcessStartTableName = "frameless_workflow_process_starts"
)

type WorkflowLockerFactory struct {
	Connection Connection
}

var _ workflow.ProcessLockers = WorkflowLockerFactory{}

func (f WorkflowLockerFactory) factory() LockerFactory[workflow.ProcessID] {
	return LockerFactory[workflow.ProcessID]{Connection: f.Connection}
}

func (f WorkflowLockerFactory) Migrate(ctx context.Context) error {
	return f.factory().Migrate(ctx)
}

func (f WorkflowLockerFactory) LockerFor(id workflow.ProcessID) guard.NonBlockingLocker {
	return f.factory().LockerFor(id)
}

func (f WorkflowLockerFactory) NonBlockingLockerFor(id workflow.ProcessID) guard.NonBlockingLocker {
	return f.factory().NonBlockingLockerFor(id)
}

type WorkflowEventRepository struct {
	Connection Connection
	Codec      workflow.Codec
}

var _ workflow.EventRepository = (*WorkflowEventRepository)(nil)
var _ comproto.OnePhaseCommitProtocol = (*WorkflowEventRepository)(nil)

func (r *WorkflowEventRepository) codec() workflow.Codec {
	if r.Codec != nil {
		return r.Codec
	}
	return wfjson.NewCodec()
}

func (r *WorkflowEventRepository) BeginTx(ctx context.Context) (context.Context, error) {
	return r.Connection.BeginTx(ctx)
}
func (r *WorkflowEventRepository) CommitTx(ctx context.Context) error {
	return r.Connection.CommitTx(ctx)
}
func (r *WorkflowEventRepository) RollbackTx(ctx context.Context) error {
	return r.Connection.RollbackTx(ctx)
}

func (r *WorkflowEventRepository) Create(ctx context.Context, ptr *workflow.Event) (rErr error) {
	if err := validateWorkflowEvent(ptr); err != nil {
		return err
	}
	event := *ptr
	data, err := r.codec().Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal workflow event: %w", err)
	}

	owned := ctx
	if _, ok := r.Connection.LookupTx(ctx); !ok {
		owned, err = r.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer comproto.FinishOnePhaseCommit(&rErr, r, owned)
	}

	_, err = r.Connection.ExecContext(owned,
		`INSERT INTO `+workflowEventTableName+` (event_id, process_id, event_type, timestamp, data) VALUES ($1, $2, $3, $4, $5)`,
		event.GetEventID(), event.GetProcessID(), event.EventType(), event.GetTimestamp(), data)
	if err != nil {
		return err
	}
	_, err = r.Connection.ExecContext(owned,
		`INSERT INTO `+workflowProcessStartTableName+` (process_id, first_event_id) VALUES ($1, $2) ON CONFLICT (process_id) DO NOTHING`,
		event.GetProcessID(), event.GetEventID())
	return err
}

func (r *WorkflowEventRepository) FindByID(ctx context.Context, id workflow.EventID) (workflow.Event, bool, error) {
	row := r.Connection.QueryRowContext(ctx, `SELECT data FROM `+workflowEventTableName+` WHERE event_id = $1`, id)
	var data []byte
	if err := row.Scan(&data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var event workflow.Event
	if err := r.codec().Unmarshal(data, &event); err != nil {
		return nil, false, err
	}
	return event, true, nil
}

func (r *WorkflowEventRepository) FindAll(ctx context.Context) iter.Seq2[workflow.Event, error] {
	return r.find(ctx, `SELECT data FROM `+workflowEventTableName+` ORDER BY event_id`)
}

func (r *WorkflowEventRepository) FindByProcessID(ctx context.Context, pid workflow.ProcessID) iter.Seq2[workflow.Event, error] {
	return r.find(ctx, `SELECT e.data FROM `+workflowEventTableName+` e JOIN `+workflowProcessStartTableName+` s ON s.process_id = e.process_id WHERE e.process_id = $1 AND e.event_id >= s.first_event_id ORDER BY e.event_id`, pid)
}

func (r *WorkflowEventRepository) find(ctx context.Context, query string, args ...any) iter.Seq2[workflow.Event, error] {
	return func(yield func(workflow.Event, error) bool) {
		rows, err := r.Connection.QueryContext(ctx, query, args...)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var data []byte
			if err := rows.Scan(&data); err != nil {
				yield(nil, err)
				return
			}
			var event workflow.Event
			if err := r.codec().Unmarshal(data, &event); err != nil {
				yield(nil, err)
				return
			}
			if !yield(event, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

func validateWorkflowEvent(ptr *workflow.Event) error {
	if ptr == nil || *ptr == nil {
		return fmt.Errorf("nil workflow event")
	}
	if (*ptr).GetEventID().IsZero() || (*ptr).GetProcessID().IsZero() || (*ptr).GetTimestamp().IsZero() {
		return fmt.Errorf("workflow event requires event ID, process ID, and timestamp")
	}
	return nil
}

func (r *WorkflowEventRepository) Migrate(ctx context.Context) error {
	return MakeMigrator(r.Connection, "frameless_workflow", migration.Steps[Connection]{
		"1": flsql.MigrationStep[Connection]{UpQuery: fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			event_id UUID PRIMARY KEY,
			process_id UUID NOT NULL,
			event_type TEXT NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL,
			data JSONB NOT NULL
		);
		CREATE INDEX IF NOT EXISTS frameless_workflow_events_process_event_idx ON %s (process_id, event_id);
		CREATE TABLE IF NOT EXISTS %s (
			process_id UUID PRIMARY KEY,
			first_event_id UUID NOT NULL
		);`, workflowEventTableName, workflowEventTableName, workflowProcessStartTableName)},
	}).Migrate(ctx)
}

// WorkflowProcessExecutionQueue is a durable PostgreSQL queue for runtime schedules.
type WorkflowProcessExecutionQueue struct {
	Queue Queue[workflow.ProcessExecution, workflow.ProcessExecution]
}

var _ workflow.ProcessExecutionQueue = (*WorkflowProcessExecutionQueue)(nil)

func (q *WorkflowProcessExecutionQueue) Publish(ctx context.Context, v workflow.ProcessExecution) error {
	return q.Queue.Publish(ctx, v)
}
func (q *WorkflowProcessExecutionQueue) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ProcessExecution] {
	return q.Queue.Subscribe(ctx)
}
func (q *WorkflowProcessExecutionQueue) Migrate(ctx context.Context) error {
	return q.Queue.Migrate(ctx)
}

// WorkflowProcessChangeBroadcast is volatile. The durable queue remains the source of truth.
type WorkflowProcessChangeBroadcast struct {
	mu     sync.RWMutex
	nextID uint64
	queues map[uint64]chan workflow.ProcessChangeEvent
}

var _ workflow.ProcessChangeBroadcast = (*WorkflowProcessChangeBroadcast)(nil)

func (b *WorkflowProcessChangeBroadcast) Publish(ctx context.Context, event workflow.ProcessChangeEvent) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.queues {
		select {
		case ch <- event:
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return nil
}

func (b *WorkflowProcessChangeBroadcast) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ProcessChangeEvent] {
	b.mu.Lock()
	if b.queues == nil {
		b.queues = make(map[uint64]chan workflow.ProcessChangeEvent)
	}
	id := b.nextID
	b.nextID++
	ch := make(chan workflow.ProcessChangeEvent, 64)
	b.queues[id] = ch
	b.mu.Unlock()

	return func(yield func(pubsub.Message[workflow.ProcessChangeEvent], error) bool) {
		defer func() {
			b.mu.Lock()
			delete(b.queues, id)
			close(ch)
			b.mu.Unlock()
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-ch:
				if !yield(pubsub.MakeMessage(ctx, event, nil, nil), nil) {
					return
				}
			}
		}
	}
}
