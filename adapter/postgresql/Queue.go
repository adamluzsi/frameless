package postgresql

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/random"
)

type Queue[Entity any] struct {
	Name       string
	Connection Connection

	// Codec serialises Entity values into the message data column
	// and deserialises them back. When nil, a default JSON codec is used.
	//
	// Default: jsonkit.Codec
	Codec codec.Codec

	// ToMeta, when set, extracts a JSON-encodable value (typically map[string]any)
	// from the Entity. The result is stored in a separate JSONB "meta" column,
	// independent from the data column, so queue ordering concerns can be expressed
	// via SortBy without coupling to the data codec.
	//
	// When nil, the meta column is NULL for every row and SortBy must be empty.
	ToMeta func(Entity) map[string]any

	// EmptyQueueBreakTime is the time.Duration that the queue waits when the queue is empty for the given queue Name.
	EmptyQueueBreakTime time.Duration
	// Blocking flag will cause the Queue.Publish method to wait until the message is processed.
	Blocking bool

	// LIFO flag will set the queue to use a Last in First out ordering
	LIFO bool
	// SortBy, when set, makes the queue order by this expression instead of created_at.
	//
	// The expression is interpolated into the ORDER BY clause, so it must
	// be a safe SQL expression over the columns of the queue table.
	// The default schema has columns: id, queue, data, meta, created_at.
	// A common pattern with ToMeta is to order by a JSONB path on the meta column,
	// e.g. `(meta->>'start_time')::timestamptz ASC NULLS LAST, created_at ASC`.
	SortBy string
}

func (q Queue[Entity]) getCodec() codec.Codec {
	if q.Codec != nil {
		return q.Codec
	}
	return &jsonkit.Codec{}
}

func (q Queue[Entity]) Purge(ctx context.Context) error {
	_, err := q.Connection.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", queueTableName))
	return err
}

func (q Queue[Entity]) Publish(ctx context.Context, v Entity) error {
	return q.PublishMany(ctx, v)
}

func (q Queue[Entity]) PublishMany(ctx context.Context, vs ...Entity) error {
	if q.Name == "" {
		return fmt.Errorf("missing queue name")
	}
	c := q.getCodec()
	var (
		rnd   = random.New(random.CryptoSeed{})
		phg   = makePrepareStatementPlaceholderGenerator()
		query string
		args  []any
		ids   []string
	)
	query += fmt.Sprintf("INSERT INTO %s (id, queue, data, meta, created_at) Values", queueTableName)
	for i, v := range vs {
		if i == 0 {
			query += "\n"
		} else {
			query += ",\n"
		}
		query += fmt.Sprintf("(%s, %s, %s, %s, %s)", phg(), phg(), phg(), phg(), phg())
		data, err := c.Marshal(v)
		if err != nil {
			return err
		}
		var meta any
		if q.ToMeta != nil {
			meta = q.ToMeta(v)
		}
		id := rnd.UUID()
		ids = append(ids, id)
		args = append(args, id, q.Name, data, meta, clock.Now().UTC())
	}

	_, err := q.Connection.ExecContext(ctx, query, args...)

	if q.Blocking {
		for {
			checkQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ANY($1)", queueTableName)
			var count int
			if err := q.Connection.QueryRowContext(ctx, checkQuery, &ids).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				break
			}
			clock.Sleep(time.Second / 3) // TODO: replace with volatile queue notify mechanism
		}
	}

	return err
}

const queueTableName = "frameless_queue_messages"

const queryCreateQueueTable = `
CREATE TABLE IF NOT EXISTS ` + queueTableName + ` (
	id         TEXT PRIMARY KEY,
    queue      TEXT NOT NULL,
	data       BYTEA NOT NULL,
	meta       JSONB,
	created_at TIMESTAMP WITH TIME ZONE
)
;`

// queryMigrateQueueAddMetaAndByteifyData brings an older queue table up to the
// current schema: adds a JSONB "meta" column (populated by ToMeta at publish
// time, used by SortBy expressions that want to order by entity fields without
// depending on the data codec), and rewrites the data column from JSON to
// BYTEA so the queue can carry payloads encoded by any codec, not just JSON.
//
// Old rows in the data column are kept byte-for-byte: their JSON text is
// reinterpreted as raw bytes. Because the default codec and the historical
// DTO shape have changed, pre-existing rows are not readable through the
// current API and must be drained before deploying.
const queryMigrateQueueAddMetaAndByteifyData = `
ALTER TABLE ` + queueTableName + `
    ADD COLUMN IF NOT EXISTS meta JSONB;
ALTER TABLE ` + queueTableName + `
    ALTER COLUMN data TYPE BYTEA USING convert_to(data::text, 'UTF8');
`

func (q Queue[Entity]) Migrate(ctx context.Context) error {
	return MakeMigrator(q.Connection, queueTableName, migration.Steps[Connection]{
		"0": flsql.MigrationStep[Connection]{UpQuery: queryCreateQueueTable},
		"1": flsql.MigrationStep[Connection]{
			UpQuery: queryMigrateQueueAddMetaAndByteifyData,
			DownQuery: `
ALTER TABLE ` + queueTableName + ` ALTER COLUMN data TYPE JSON USING data::text::json;
ALTER TABLE ` + queueTableName + ` DROP COLUMN IF EXISTS meta;
`,
		},
	}).Migrate(ctx)
}

func (q Queue[Entity]) Subscribe(ctx context.Context) pubsub.Subscription[Entity] {
	return iterkit.From(func(yield func(pubsub.Message[Entity]) bool) (rErr error) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := q.Connection.DB.Ping(ctx); err != nil {
			return err
		}
		sub := &queueSubscription[Entity]{
			Queue: q,
			CTX:   ctx,
		}
		defer errorkit.Finish(&rErr, sub.Close)
		for sub.Next() {
			if !yield(sub.Value()) {
				return nil
			}
		}
		return sub.Err()
	})
}

type queueSubscription[Entity any] struct {
	CTX   context.Context
	Queue Queue[Entity]

	idle   int32
	closed bool
	err    error
	value  *queueMessage[Entity]
}

func (qs *queueSubscription[Entity]) IsIdle() bool {
	return atomic.LoadInt32(&qs.idle) == 0
}

func (qs *queueSubscription[Entity]) Close() error {
	if qs.value != nil {
		_ = qs.value.NACK()
	}
	qs.closed = true
	return nil
}

func (qs *queueSubscription[Entity]) Err() error {
	return qs.err
}

func (qs *queueSubscription[Entity]) orderingExpr() string {
	if qs.Queue.SortBy != "" {
		// SortBy is trusted; it is interpolated as-is into the ORDER BY clause.
		// Callers must pass a safe SQL expression over the queue table's columns
		// (id, queue, data, meta, created_at).
		return qs.Queue.SortBy
	}
	var ordering = "ASC"
	if qs.Queue.LIFO {
		ordering = "DESC"
	}
	return "created_at " + ordering
}

const queryQueuePopMessage = `
DELETE FROM ` + queueTableName + `
    WHERE id = (
      SELECT id
      FROM ` + queueTableName + `
      WHERE queue = $1
      ORDER BY %s
      FOR UPDATE SKIP LOCKED
      LIMIT 1
    )
    RETURNING id, data;
`

func (qs *queueSubscription[Entity]) Next() bool {
fetch:

	if err := qs.CTX.Err(); err != nil {
		return false
	}

	if qs.closed {
		return false
	}

	if qs.err != nil {
		return false
	}

	atomic.StoreInt32(&qs.idle, 1)

	if qs.value != nil {
		_ = qs.value.NACK()
		qs.value = nil
	}

	tx, err := qs.Queue.Connection.BeginTx(qs.CTX)
	if err != nil {
		if errors.Is(err, qs.CTX.Err()) {
			return false
		}
		qs.err = err
		return false
	}

	var (
		row = qs.Queue.Connection.QueryRowContext(tx,
			fmt.Sprintf(queryQueuePopMessage, qs.orderingExpr()),
			qs.Queue.Name)
		id   string
		data []byte
	)
	if err := row.Scan(&id, &data); err != nil {
		_ = qs.Queue.Connection.RollbackTx(contextkit.Detach(tx))
		if errors.Is(err, qs.CTX.Err()) {
			return false
		}
		if errors.Is(err, errNoRows) {
			atomic.StoreInt32(&qs.idle, 0)
			select {
			case <-qs.CTX.Done():
				return false
			case <-clock.After(qs.getEmptyQueueBreakTime()):
				goto fetch
			}
		}
		qs.err = err
		return false
	}

	var ent Entity
	if err := qs.Queue.getCodec().Unmarshal(data, &ent); err != nil {
		_ = qs.Queue.Connection.RollbackTx(contextkit.Detach(tx))
		qs.err = err
		return false
	}

	qs.value = &queueMessage[Entity]{
		q:    qs.Queue,
		tx:   tx,
		data: ent,
	}
	return true
}

func (qs *queueSubscription[Entity]) Value() pubsub.Message[Entity] {
	return qs.value
}

func (qs *queueSubscription[Entity]) getEmptyQueueBreakTime() time.Duration {
	const defaultBreakTime = 42 * time.Millisecond
	if qs.Queue.EmptyQueueBreakTime == 0 {
		return defaultBreakTime
	}
	return qs.Queue.EmptyQueueBreakTime
}

type queueMessage[Entity any] struct {
	q    Queue[Entity]
	tx   context.Context
	data Entity
}

func (qm queueMessage[Entity]) Context() context.Context {
	return qm.tx
}

func (qm queueMessage[Entity]) ACK() error {
	// when context cancellation happens,
	// the already received message should be still ACK able
	// Thus detaching from cancellation is acceptable
	return qm.q.Connection.CommitTx(contextkit.Detach(qm.tx))
}

func (qm queueMessage[Entity]) NACK() error {
	// when context cancellation happens,
	// the already received message should be still ACK able
	// Thus detaching from cancellation is acceptable
	return qm.q.Connection.RollbackTx(contextkit.Detach(qm.tx))
}

func (qm queueMessage[Entity]) Data() Entity {
	return qm.data
}
