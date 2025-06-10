package postgresql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/random"
)

type Queue[Entity, JSONDTO any] struct {
	Name       string
	Connection Connection
	Mapping    dtokit.MapperTo[Entity, JSONDTO]

	// EmptyQueueBreakTime is the time.Duration that the queue waits when the queue is empty for the given queue Name.
	EmptyQueueBreakTime time.Duration
	// Blocking flag will cause the Queue.Publish method to wait until the message is processed.
	Blocking bool

	// LIFO flag will set the queue to use a Last in First out ordering
	LIFO bool
}

type QueueMapper[ENT, DTO any] interface {
	ToDTO(ent ENT) (DTO, error)
	ToEnt(dto DTO) (ENT, error)
}

func (q Queue[Entity, JSONDTO]) Purge(ctx context.Context) error {
	_, err := q.Connection.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", queueTableName))
	return err
}

func (q Queue[Entity, JSONDTO]) Publish(ctx context.Context, vs ...Entity) error {
	if len(vs) == 0 {
		return ctx.Err()
	}
	if q.Name == "" {
		return fmt.Errorf("missing queue name")
	}
	var (
		rnd   = random.New(random.CryptoSeed{})
		phg   = makePrepareStatementPlaceholderGenerator()
		query string
		args  []any
		ids   []string
	)
	query += fmt.Sprintf("INSERT INTO %s (id, queue, data, created_at) Values", queueTableName)
	for i, v := range vs {
		if i == 0 {
			query += "\n"
		} else {
			query += ",\n"
		}
		query += fmt.Sprintf("(%s, %s, %s, %s)", phg(), phg(), phg(), phg())
		dto, err := q.Mapping.MapToDTO(ctx, v)
		if err != nil {
			return err
		}
		data, err := json.Marshal(dto)
		if err != nil {
			return err
		}
		id := rnd.UUID()
		ids = append(ids, id)
		args = append(args, id, q.Name, data, clock.Now().UTC())
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
	data       JSON NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE
)
;`

func (q Queue[Entity, JSONDTO]) Migrate(ctx context.Context) error {
	return MakeMigrator(q.Connection, queueTableName, migration.Steps[Connection]{
		"0": flsql.MigrationStep[Connection]{UpQuery: queryCreateQueueTable},
	}).Migrate(ctx)
}

func (q Queue[Entity, JSONDTO]) Subscribe(ctx context.Context) (pubsub.Subscription[Entity], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := q.Connection.DB.Ping(ctx); err != nil {
		return nil, err
	}
	return iterkit.FromPullIter(&queueSubscription[Entity, JSONDTO]{
		Queue: q,
		CTX:   ctx,
	}), nil
}

type queueSubscription[Entity, JSONDTO any] struct {
	CTX   context.Context
	Queue Queue[Entity, JSONDTO]

	idle   int32
	closed bool
	err    error
	value  *queueMessage[Entity, JSONDTO]
}

func (qs *queueSubscription[Entity, JSONDTO]) IsIdle() bool {
	return atomic.LoadInt32(&qs.idle) == 0
}

func (qs *queueSubscription[Entity, JSONDTO]) Close() error {
	if qs.value != nil {
		_ = qs.value.NACK()
	}
	qs.closed = true
	return nil
}

func (qs *queueSubscription[Entity, JSONDTO]) Err() error {
	return qs.err
}

const queryQueuePopMessage = `
DELETE FROM ` + queueTableName + ` 
    WHERE id = (
      SELECT id
      FROM ` + queueTableName + `
      WHERE queue = $1
      ORDER BY created_at %s 
      FOR UPDATE SKIP LOCKED
      LIMIT 1
    )
    RETURNING id, data;
`

func (qs *queueSubscription[Entity, JSONDTO]) Next() bool {
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

	var ordering = "ASC"
	if qs.Queue.LIFO {
		ordering = "DESC"
	}

	var (
		row  = qs.Queue.Connection.QueryRowContext(tx, fmt.Sprintf(queryQueuePopMessage, ordering), qs.Queue.Name)
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

	var dto JSONDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		_ = qs.Queue.Connection.RollbackTx(contextkit.Detach(tx))
		qs.err = err
		return false
	}

	ent, err := qs.Queue.Mapping.MapToENT(qs.CTX, dto)
	if err != nil {
		_ = qs.Queue.Connection.RollbackTx(contextkit.Detach(tx))
		qs.err = err
		return false
	}

	qs.value = &queueMessage[Entity, JSONDTO]{
		q:    qs.Queue,
		tx:   tx,
		data: ent,
	}
	return true
}

func (qs *queueSubscription[Entity, JSONDTO]) Value() pubsub.Message[Entity] {
	return qs.value
}

func (qs *queueSubscription[Entity, JSONDTO]) getEmptyQueueBreakTime() time.Duration {
	const defaultBreakTime = 42 * time.Millisecond
	if qs.Queue.EmptyQueueBreakTime == 0 {
		return defaultBreakTime
	}
	return qs.Queue.EmptyQueueBreakTime
}

type queueMessage[Entity, JSONDTO any] struct {
	q    Queue[Entity, JSONDTO]
	tx   context.Context
	data Entity
}

func (qm queueMessage[Entity, JSONDTO]) Context() context.Context {
	return qm.tx
}

func (qm queueMessage[Entity, JSONDTO]) ACK() error {
	// when context cancellation happens,
	// the already received message should be still ACK able
	// Thus detaching from cancellation is acceptable
	return qm.q.Connection.CommitTx(contextkit.Detach(qm.tx))
}

func (qm queueMessage[Entity, JSONDTO]) NACK() error {
	// when context cancellation happens,
	// the already received message should be still ACK able
	// Thus detaching from cancellation is acceptable
	return qm.q.Connection.RollbackTx(contextkit.Detach(qm.tx))
}

func (qm queueMessage[Entity, JSONDTO]) Data() Entity {
	return qm.data
}
