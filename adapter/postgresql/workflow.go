package postgresql

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.llib.dev/frameless/pkg/flsql"

	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfjson"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/pubsub"
)

const workflowEventTableName = "frameless_workflow_events"

type WorkflowProcessLocks struct {
	Connection Connection
}

var _ workflow.ProcessLocks = WorkflowProcessLocks{}

func (f WorkflowProcessLocks) factory() LockerFactory[workflow.ProcessID] {
	return LockerFactory[workflow.ProcessID]{Connection: f.Connection}
}

func (f WorkflowProcessLocks) Migrate(ctx context.Context) error {
	return f.factory().Migrate(ctx)
}

func (f WorkflowProcessLocks) LockerFor(id workflow.ProcessID) workflow.Lock {
	return f.factory().LockerFor(id)
}

type WorkflowEventRepository struct {
	Connection Connection
	Codec      workflow.Codec
}

var _ workflow.EventRepository = (*WorkflowEventRepository)(nil)
var _ comproto.OnePhaseCommitProtocol = (*WorkflowEventRepository)(nil)

var defaultWorkflowEventCodec = wfjson.NewCodec()

func (r *WorkflowEventRepository) codec() workflow.Codec {
	if r.Codec != nil {
		return r.Codec
	}
	return defaultWorkflowEventCodec
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

	tx := ctx
	if _, ok := r.Connection.LookupTx(ctx); !ok {
		tx, err = r.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer comproto.FinishOnePhaseCommit(&rErr, r, tx)
	}

	_, err = r.Connection.ExecContext(tx,
		`INSERT INTO `+workflowEventTableName+` (process_id, event_id, data) VALUES ($1, $2, $3)`,
		event.GetProcessID(), event.GetEventID(), data)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return crud.ErrAlreadyExists
		}
	}

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
	return r.find(ctx, `SELECT data FROM `+workflowEventTableName+` WHERE process_id = $1 ORDER BY event_id`, pid)
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
	if err := workflow.ValidateEventID((*ptr).GetEventID()); err != nil {
		return err
	}
	if err := workflow.ValidateProcessID((*ptr).GetProcessID()); err != nil {
		return err
	}
	return nil
}

const workflowEventRepositoryMigrationQ1Up = `CREATE TABLE ` + workflowEventTableName + ` (
    process_id  UUID        NOT NULL,
    event_id    UUID        NOT NULL,
    data        JSONB       NOT NULL,
    CONSTRAINT ` + workflowEventTableName + `_pkey PRIMARY KEY (process_id, event_id)
)`

func (r *WorkflowEventRepository) Migrate(ctx context.Context) error {
	return MakeMigrator(r.Connection, "frameless_workflow", migration.Steps[Connection]{
		"001": flsql.MigrationStep[Connection]{
			UpQuery:   workflowEventRepositoryMigrationQ1Up,
			DownQuery: fmt.Sprintf("DROP TABLE IF EXISTS %s", workflowEventTableName),
		},
		// Storage strategy for `data`: stay inline, so reading a process history
		// never costs a TOAST lookup per event. All three parts matter:
		//
		//   MAIN     only externalised when the row cannot fit a page at all.
		//   default  toast_tuple_target left alone on purpose. It gates compression
		//            too, so raising it just stores JSONB raw: ~4.5 KB events drop
		//            from 10 rows/page to 1 and save zero TOAST lookup (PG18).
		//   pglz     kept over lz4. JSONB repeats its key names, so pglz wins on
		//            ratio (11.5x vs 9.7x at 86 KB), and MAIN decides inline vs
		//            external on the *compressed* size: at 86 KB pglz stays inline,
		//            lz4 does not.
		//
		// SET STORAGE only affects new tuples, fine for an append-only log. RESET is
		// declarative so the step converges whatever the reloption was set to before.
		"002": flsql.MigrationStep[Connection]{
			UpQuery: `ALTER TABLE ` + workflowEventTableName + `
				ALTER COLUMN data SET STORAGE MAIN,
				RESET (toast_tuple_target)`,
			DownQuery: `ALTER TABLE ` + workflowEventTableName + `
				ALTER COLUMN data SET STORAGE EXTENDED`,
		},
		// event_id is the second PK column, so lookups and prunes that only know the
		// event id cannot use the primary key and fall back to a sequential scan.
		// This index serves FindByID, and prune sweeps that range-scan the UUIDv7
		// time prefix (`WHERE event_id < <cutoff v7>`).
		//
		// UNIQUE because an EventID is a globally unique UUIDv7. Without it the same
		// event id could be stored twice under different process ids, which would
		// make FindByID ambiguous and hide the duplicate from Create's 23505 check.
		"003": flsql.MigrationStep[Connection]{
			UpQuery: `CREATE UNIQUE INDEX ` + workflowEventTableName + `_event_id_idx
				ON ` + workflowEventTableName + ` (event_id)`,
			DownQuery: `DROP INDEX IF EXISTS ` + workflowEventTableName + `_event_id_idx`,
		},
	}).Migrate(ctx)
}

// WorkflowQueue is a durable PostgreSQL queue for runtime schedules.
// Delivered message contexts retain the subscription context, not the internal
// delivery transaction: handler work must not depend on ACK/NACK to commit.
type WorkflowQueue struct {
	Connection Connection
	// Codec is the workflow entity codec
	//
	// Default: wfjson.Codec
	Codec workflow.Codec
	// Name [optional] is the WorkflowQueue's table name.
	//
	// Default: frameless_workflow_queue
	Name string
	o    sync.Once
	q    Queue[workflow.ExecutionRequest]
}

var _ workflow.Queue = (*WorkflowQueue)(nil)

func (q *WorkflowQueue) init() {
	q.o.Do(func() {
		var name = q.Name
		if name == "" {
			const DefaultName = "frameless_workflow"
			name = DefaultName
		}

		var codec = q.Codec
		if codec == nil {
			codec = wfjson.NewCodec()
		}

		q.q = Queue[workflow.ExecutionRequest]{
			Connection: q.Connection,
			Table:      name,
			Codec:      codec,
			// wfcontract.Queue asserts items come out ordered by
			// ExecutionRequest.StartTime ascending. The StartTime is
			// exposed via the JSONB meta column as `start_time` and the
			// ORDER BY clause reads from there so the order does not
			// depend on the payload codec.
			ToMeta: func(req workflow.ExecutionRequest) map[string]any {
				return map[string]any{"start_time": req.StartTime}
			},
			SortBy: `(meta->>'start_time')::timestamptz ASC NULLS LAST, created_at ASC`,
		}
	})
}

func (q *WorkflowQueue) Publish(ctx context.Context, v workflow.ExecutionRequest) error {
	q.init()
	return q.q.Publish(ctx, v)
}

func (q *WorkflowQueue) Subscribe(ctx context.Context) pubsub.Subscription[workflow.ExecutionRequest] {
	q.init()
	return func(yield func(pubsub.Message[workflow.ExecutionRequest], error) bool) {
		for msg, err := range q.q.Subscribe(ctx) {
			if msg != nil {
				// Keep V1's delivery transaction private to ACK/NACK. Workflow
				// rescheduling and event writes must survive a delivery's NACK.
				msg = workflowQueueMessage{Message: msg, ctx: ctx}
			}
			if !yield(msg, err) {
				return
			}
		}
	}
}

type workflowQueueMessage struct {
	pubsub.Message[workflow.ExecutionRequest]
	ctx context.Context
}

func (m workflowQueueMessage) Context() context.Context { return m.ctx }

func (q *WorkflowQueue) Migrate(ctx context.Context) error {
	q.init()
	return q.q.Migrate(ctx)
}

// WorkflowNotificationBroadcast delegates to Broadcast[workflow.Notification]
// with the workflow channel name and wfjson codec defaults. Its public fields
// mirror Broadcast, including the optional stateless subscription configuration.
// Configure a value before first use; do not copy or modify it afterwards.
type WorkflowNotificationBroadcast struct {
	Connection Connection
	Name       string
	Codec      workflow.Codec

	// StatelessSubscribe enables table-backed polling when non-nil; nil uses LISTEN.
	// An empty config uses the default polling interval and subscriber lease.
	// Registration and heartbeats start at Subscribe, not at iteration. Cancel the
	// subscription context even if you never iterate.
	StatelessSubscribe *BroadcastStatelessSubscribe

	o sync.Once
	b Broadcast[workflow.Notification]
}

const workflowNotificationBroadcastDefaultName = "frameless_workflow_notifications"

var _ workflow.NotificationBroadcast = (*WorkflowNotificationBroadcast)(nil)

func (b *WorkflowNotificationBroadcast) init() {
	b.o.Do(func() {
		name := b.Name
		if strings.TrimSpace(name) == "" {
			name = workflowNotificationBroadcastDefaultName
		}
		if b.Codec == nil {
			b.Codec = wfjson.NewCodec()
		}
		b.b = Broadcast[workflow.Notification]{
			Connection:         b.Connection,
			Name:               name,
			Codec:              b.Codec,
			StatelessSubscribe: b.StatelessSubscribe,
		}
	})
}

func (b *WorkflowNotificationBroadcast) Publish(ctx context.Context, event workflow.Notification) error {
	b.init()
	return b.b.Publish(ctx, event)
}

func (b *WorkflowNotificationBroadcast) Subscribe(ctx context.Context) pubsub.Subscription[workflow.Notification] {
	b.init()
	return b.b.Subscribe(ctx)
}

func (b *WorkflowNotificationBroadcast) Migrate(ctx context.Context) error {
	b.init()
	return b.b.Migrate(ctx)
}
