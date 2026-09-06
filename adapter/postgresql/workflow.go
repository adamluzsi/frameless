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
	"go.llib.dev/frameless/pkg/logger"
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

const workflowQueueDefaultName = "frameless_workflow_queue"

var _ workflow.Queue = (*WorkflowQueue)(nil)

func (q *WorkflowQueue) init() {
	q.o.Do(func() {
		var name = q.Name
		if name == "" {
			name = workflowQueueDefaultName
		}

		var codec = q.Codec
		if codec == nil {
			codec = wfjson.NewCodec()
		}

		q.q = Queue[workflow.ExecutionRequest]{
			Name:       name,
			Connection: q.Connection,
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
	return q.q.Subscribe(ctx)
}

func (q *WorkflowQueue) Migrate(ctx context.Context) error {
	q.init()
	return q.q.Migrate(ctx)
}

// WorkflowNotificationBroadcast publishes workflow notifications using
// PostgreSQL's LISTEN/NOTIFY so that worker nodes on different processes
// (and on different hosts) can be notified about queue changes in real time.
//
// The on-wire notification payload is owned by pkg/workflow/wfjson, which keeps
// the format identical to the durable event log and the runtime's codec.
type WorkflowNotificationBroadcast struct {
	Connection Connection
	Name       string
	Codec      workflow.Codec

	o       sync.Once
	channel string
}

const workflowNotificationBroadcastDefaultName = "frameless_workflow_notifications"

var _ workflow.NotificationBroadcast = (*WorkflowNotificationBroadcast)(nil)

func (b *WorkflowNotificationBroadcast) init() {
	b.o.Do(func() {
		name := b.Name
		if name == "" {
			name = workflowNotificationBroadcastDefaultName
		}
		// PostgreSQL identifiers are lowercase-folded; lowercasing the name
		// here gives a stable, predictable channel even if the caller passes
		// a mixed-case Name. We also strip characters that are not legal in
		// a PostgreSQL identifier so the channel name is always safe.
		b.channel = sanitizePGListenChannel(name)
		if b.Codec == nil {
			b.Codec = wfjson.NewCodec()
		}
	})
}

func (b *WorkflowNotificationBroadcast) Publish(ctx context.Context, event workflow.Notification) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b.init()

	payload, err := b.Codec.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal workflow notification: %w", err)
	}

	// pg_notify is delivered to listeners even when the publisher is inside
	// a transaction; raw NOTIFY would not be, so we use the function form.
	_, err = b.Connection.ExecContext(ctx, `SELECT pg_notify($1, $2)`, b.channel, payload)
	return err
}

func (b *WorkflowNotificationBroadcast) Subscribe(ctx context.Context) pubsub.Subscription[workflow.Notification] {
	b.init()

	// LISTEN requires a dedicated session. Acquiring a *pgxpool.Conn reserves
	// one from the pool for the lifetime of the subscription.
	rawConn, err := b.Connection.DB.Acquire(ctx)
	if err != nil {
		return func(yield func(pubsub.Message[workflow.Notification], error) bool) {
			yield(nil, err)
		}
	}

	pgxConn := rawConn.Conn()

	if _, err := pgxConn.Exec(ctx, fmt.Sprintf(`LISTEN %s`, pgxIdentifier(b.channel))); err != nil {
		rawConn.Release()
		return func(yield func(pubsub.Message[workflow.Notification], error) bool) {
			yield(nil, err)
		}
	}

	// Derive a cancellable context for the waiter goroutine so we can unblock
	// WaitForNotification deterministically when the subscription ends, even if
	// the caller's ctx is not canceled.
	waitCtx, cancelWait := context.WithCancel(ctx)

	// Run WaitForNotification in a dedicated goroutine and bridge it with a
	// buffered channel. This makes Subscribe responsive to context
	// cancellation regardless of whether WaitForNotification itself respects
	// ctx promptly, and it also keeps the dedicated connection alive for the
	// lifetime of the subscription.
	notifications := make(chan *pgconn.Notification, 32)
	waitErr := make(chan error, 1)
	stopWaiter := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer close(notifications)
		for {
			select {
			case <-stopWaiter:
				return
			default:
			}
			n, err := pgxConn.WaitForNotification(waitCtx)
			if err != nil {
				select {
				case waitErr <- err:
				case <-stopWaiter:
				}
				return
			}
			select {
			case notifications <- n:
			case <-stopWaiter:
				return
			}
		}
	}()

	// closeStopWaiter is idempotent so that a misuse of the returned iterator
	// (calling the subscription function more than once) cannot panic with
	// "close of closed channel".
	var closeStop sync.Once
	closeStopFn := func() { closeStop.Do(func() { close(stopWaiter) }) }

	return func(yield func(pubsub.Message[workflow.Notification], error) bool) {
		defer func() {
			closeStopFn()
			// Unblock WaitForNotification if it is still parked on the
			// dedicated connection. The pgx contextWatcher issues a backend
			// cancel in the wake so the goroutine returns promptly.
			cancelWait()
			<-done
			// pgxpool does not call DISCARD ALL on Release, so without an
			// explicit UNLISTEN the LISTEN registration would survive on the
			// pooled connection. Any notifications published while the conn is
			// idle in the pool would then be queued for that backend and
			// replayed to whichever subscriber later acquires it, breaking
			// the volatile contract.
			if _, err := pgxConn.Exec(context.Background(), `UNLISTEN *`); err != nil {
				logger.Error(ctx, err.Error())
			}
			rawConn.Release()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case err := <-waitErr:
				if ctx.Err() != nil {
					return
				}
				yield(nil, err)
				return
			case n, ok := <-notifications:
				if !ok {
					return
				}
				if n == nil || n.Channel != b.channel {
					continue
				}

				var event workflow.Notification
				if err := b.Codec.Unmarshal([]byte(n.Payload), &event); err != nil {
					yield(nil, fmt.Errorf("decode workflow notification: %w", err))
					continue
				}
				if !yield(pubsub.MakeMessage(ctx, event, nil, nil), nil) {
					return
				}
			}
		}
	}
}

// pgxIdentifier quotes a channel name so it is safe to interpolate into a
// LISTEN statement. PostgreSQL does not accept bind parameters on LISTEN/UNLISTEN.
func pgxIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sanitizePGListenChannel(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return workflowNotificationBroadcastDefaultName
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return workflowNotificationBroadcastDefaultName
	}
	if out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	return out
}
