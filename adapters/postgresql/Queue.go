package postgresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/testcase/clock"
	"github.com/adamluzsi/testcase/random"
	"github.com/lib/pq"
	"time"
)

type Queue[Entity, JSONDTO any] struct {
	Name              string
	ConnectionManager ConnectionManager
	Mapping           QueueMapping[Entity, JSONDTO]

	// Blocking flag will cause the Queue.Publish method to wait until the message is processed.
	Blocking bool
	// LIFO flag will set the queue to use a Last in First out ordering
	LIFO bool
}

type QueueMapping[ENT, DTO any] interface {
	ToDTO(ent ENT) (DTO, error)
	ToEnt(dto DTO) (ENT, error)
}

func (q Queue[Entity, JSONDTO]) Purge(ctx context.Context) error {
	connection, err := q.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}
	_, err = connection.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s", queueTableName))
	return err
}

func (q Queue[Entity, JSONDTO]) Publish(ctx context.Context, vs ...Entity) error {
	if 0 == len(vs) {
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
		dto, err := q.Mapping.ToDTO(v)
		if err != nil {
			return err
		}
		data, err := json.Marshal(dto)
		if err != nil {
			return err
		}
		id := rnd.UUID()
		ids = append(ids, id)
		args = append(args, id, q.Name, data, clock.TimeNow().UTC())
	}

	connection, err := q.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}

	_, err = connection.ExecContext(ctx, query, args...)

	if q.Blocking {
		for { // TODO: replace with volatile queue notify mechanism
			checkQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = ANY($1)", queueTableName)
			var count int
			if err := connection.QueryRowContext(ctx, checkQuery, pq.Array(ids)).Scan(&count); err != nil {
				return err
			}
			if count == 0 {
				break
			}
			clock.Sleep(500 * time.Millisecond)
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

var queueMigratorConfig = MigratorConfig{
	Namespace: queueTableName,
	Steps: []MigratorStep{
		MigrationStep{UpQuery: queryCreateQueueTable},
	},
}

func (q Queue[Entity, JSONDTO]) Migrate(ctx context.Context) error {
	cm, ok := q.ConnectionManager.(*connectionManager)
	if !ok {
		return fmt.Errorf("migration with connection manager is not supported")
	}
	return Migrator{
		DB:     cm.DB,
		Config: queueMigratorConfig,
	}.Up(ctx)
}

func (q Queue[Entity, JSONDTO]) Subscribe(ctx context.Context) iterators.Iterator[pubsub.Message[Entity]] {
	return &queueSubscription[Entity, JSONDTO]{
		Queue: q,
		CTX:   ctx,
	}
}

type queueSubscription[Entity, JSONDTO any] struct {
	CTX   context.Context
	Queue Queue[Entity, JSONDTO]

	closed bool
	err    error
	value  *queueMessage[Entity, JSONDTO]
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

	if qs.value != nil {
		_ = qs.value.NACK()
		qs.value = nil
	}

	tx, err := qs.Queue.ConnectionManager.BeginTx(qs.CTX)
	if err != nil {
		if errors.Is(err, qs.CTX.Err()) {
			return false
		}
		qs.err = err
		return false
	}

	connection, err := qs.Queue.ConnectionManager.Connection(tx)
	if err != nil {
		_ = qs.Queue.ConnectionManager.RollbackTx(tx)
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

	row := connection.QueryRowContext(tx, fmt.Sprintf(queryQueuePopMessage, ordering))
	if err := row.Err(); err != nil {
		_ = qs.Queue.ConnectionManager.RollbackTx(tx)
		if errors.Is(err, qs.CTX.Err()) {
			return false
		}
		qs.err = err
		return false
	}

	var (
		id   string
		data []byte
	)
	if err := row.Scan(&id, &data); err != nil {
		_ = qs.Queue.ConnectionManager.RollbackTx(tx)
		if errors.Is(err, qs.CTX.Err()) {
			return false
		}
		if errors.Is(err, sql.ErrNoRows) {
			clock.Sleep(time.Microsecond)
			goto fetch
		}
		qs.err = err
		return false
	}

	var dto JSONDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		_ = qs.Queue.ConnectionManager.RollbackTx(tx)
		qs.err = err
		return false
	}

	ent, err := qs.Queue.Mapping.ToEnt(dto)
	if err != nil {
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

type queueMessage[Entity, JSONDTO any] struct {
	q    Queue[Entity, JSONDTO]
	tx   context.Context
	data Entity
}

func (qm queueMessage[Entity, JSONDTO]) ACK() error {
	return qm.q.ConnectionManager.CommitTx(qm.tx)
}

func (qm queueMessage[Entity, JSONDTO]) NACK() error {
	return qm.q.ConnectionManager.RollbackTx(qm.tx)
}

func (qm queueMessage[Entity, JSONDTO]) Data() Entity {
	return qm.data
}
