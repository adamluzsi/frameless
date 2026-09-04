package memory

import (
	"context"
	"fmt"
	"iter"
	"math"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/txkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/testcase/clock"
)

type Queue[Data any] struct {
	// LIFO is a flag to change element ordering from FIFO to LIFO
	LIFO bool
	// Volatile will flag the Queue to act like a Volatile queue
	Volatile bool
	// Blocking will cause the Queue to wait until the published messages are ACK -ed.
	// A blocking queue is not compatible with transactions and will error on them.
	Blocking bool
	// SortLessFunc will define how to sort data, when we look for what message to handle next.
	// if not supplied FIFO is the default ordering.
	SortLessFunc func(i Data, j Data) bool

	m    sync.RWMutex
	msgs []*queueMessage[Data]
	subs map[subscriptionID]*QueueSubscription[Data]
}

func (q *Queue[Data]) Publish(ctx context.Context, data Data) (rErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if q.Blocking {
		if _, ok := q.txm().LookupTx(ctx); ok {
			return errBlockingQueueCannotBeUsedInTransaction
		}
		return q.blockingWait(ctx, data)
	}
	return q.txm().Q(ctx).Publish(ctx, data)
}

var msgIDIndex uint64

func (q *Queue[Data]) publish(ctx context.Context, data Data) *queueMessage[Data] {
	q.m.Lock()
	defer q.m.Unlock()
	var msg = &queueMessage[Data]{
		q:         q,
		v:         data,
		id:        fmt.Sprintf("%s-%d", rnd.UUID(), atomic.AddUint64(&msgIDIndex, 1)),
		timestamp: clock.Now(),
	}
	q.msgs = append(q.msgs, msg)
	return msg
}
func (q *Queue[Data]) blockingWait(ctx context.Context, data Data) error {
	// wait until the published message is acknowledged,
	// which is signalled by the message no longer being present in the queue.
	var target = q.publish(ctx, data)
	for ctx.Err() == nil {
		var found bool
		for msg := range q.rIter() {
			if msg.id == target.id {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		runtime.Gosched()
	}
	return ctx.Err()
}

func (q *Queue[Data]) rIter() iter.Seq[*queueMessage[Data]] {
	return func(yield func(*queueMessage[Data]) bool) {
		q.m.Lock()
		q.sort(q.msgs)
		q.m.Unlock()

		q.m.RLock()
		defer q.m.RUnlock()
		for _, msg := range q.msgs {
			if !yield(msg) {
				return
			}
		}
	}
}

func (q *Queue[Data]) take(ctx context.Context, s *QueueSubscription[Data]) (_ *queueMessage[Data], _ context.Context, ack, nack func() error, _ error) {
do:
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, nil, err
	}

	msgs := q.rIter()

	if q.Volatile {
		msgs = iterkit.Filter(msgs, func(qm *queueMessage[Data]) bool {
			return s.createdAt.Before(qm.timestamp) || s.createdAt.Equal(qm.timestamp)
		})
	}

	// TransactionalMessageContext support
	var tx context.Context = ctx
	var txCommit, txRollback func() error
	txCommit = func() error { return nil }
	txRollback = func() error { return nil }
	if !q.Blocking {
		var err error
		tx, err = q.BeginTx(tx)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		// Use a detached context for commit/rollback so that subscription
		// cancellation does not interfere with the transaction lifecycle.
		// The detached context still carries all values (including the tx)
		// but ignores the parent's cancellation.
		txForTxOps := contextkit.WithoutCancel(tx)
		txCommit = func() error {
			return q.CommitTx(txForTxOps)
		}
		txRollback = func() error {
			return q.RollbackTx(txForTxOps)
		}
	}

	for msg := range msgs {
		if msg.take(s.id) {
			var ack, nack func() error

			ack = func() error {
				if err := txCommit(); err != nil {
					// Transaction commit failed, so the message should be released
					// so it can be picked up again. We cannot call nack() because
					// the transaction is already marked as done by the failed commit.
					q.m.Lock()
					msg.release(s.id)
					q.m.Unlock()
					return err
				}
				q.m.Lock()
				defer q.m.Unlock()
				q.msgs = slicekit.Filter(q.msgs, func(m *queueMessage[Data]) bool {
					return m.id != msg.id
				})
				msg.release(s.id)
				return nil
			}

			nack = func() error {
				q.m.Lock()
				defer q.m.Unlock()
				msg.release(s.id)
				return txRollback()
			}

			return msg, tx, ack, nack, nil
		}

		runtime.Gosched()
	}

	_ = txRollback()
	goto do
}

func (q *Queue[Data]) sort(recs []*queueMessage[Data]) {
	sort.Slice(recs, func(i, j int) bool {
		if q.SortLessFunc != nil {
			return q.SortLessFunc(recs[i].v, recs[j].v)
		}
		less := recs[i].timestamp.Before(recs[j].timestamp)
		if q.LIFO {
			return !less
		}
		return less
	})
}

type queueTx[Data any] struct {
	m sync.Mutex
	q *Queue[Data]

	ds   []Data
	done bool
}

type qpub[Data any] struct {
	Publish func(ctx context.Context, data Data) error
}

type qPublisher[Data any] func(ctx context.Context, data Data) error

var _ pubsub.Publisher[int] = (*qPublisher[int])(nil)

func (fn qPublisher[Data]) Publish(ctx context.Context, data Data) error {
	return fn(ctx, data)
}

func (q *Queue[Data]) dbPublish(ds ...Data) error {
	return q.dbPublishMessages(slicekit.Map(ds, func(v Data) *queueMessage[Data] {
		return &queueMessage[Data]{
			q:         q,
			v:         v,
			id:        fmt.Sprintf("%s-%d", rnd.UUID(), atomic.AddUint64(&msgIDIndex, 1)),
			timestamp: clock.Now(),
		}
	})...)
}

func (q *Queue[Data]) dbPublishMessages(msgs ...*queueMessage[Data]) error {
	q.m.Lock()
	defer q.m.Unlock()
	q.msgs = append(q.msgs, msgs...)
	return nil
}

func (q *Queue[Data]) txm() txkit.Manager[Queue[Data], queueTx[Data], qPublisher[Data]] {
	return txkit.Manager[Queue[Data], queueTx[Data], qPublisher[Data]]{
		DB: q,
		TxAdapter: func(tx *queueTx[Data]) qPublisher[Data] {
			return func(ctx context.Context, data Data) error {
				tx.m.Lock()
				defer tx.m.Unlock()
				tx.ds = append(tx.ds, data)
				return nil
			}
		},
		DBAdapter: func(db *Queue[Data]) qPublisher[Data] {
			return qPublisher[Data](func(ctx context.Context, ds Data) error {
				return db.dbPublish(ds)
			})
		},
		Begin: func(ctx context.Context, db *Queue[Data]) (*queueTx[Data], error) {
			return &queueTx[Data]{q: db, ds: []Data{}}, nil
		},
		Commit: func(ctx context.Context, tx *queueTx[Data]) error {
			tx.m.Lock()
			defer tx.m.Unlock()
			// Flush the buffered data directly to the underlying queue.
			// We must not go through tx.q.Publish here, because the context
			// still carries this transaction, which would route the publish
			// back into the tx buffer and re-acquire tx.m, causing a deadlock.
			return tx.q.dbPublish(tx.ds...)
		},
		Rollback: func(ctx context.Context, tx *queueTx[Data]) error {
			tx.m.Lock()
			defer tx.m.Unlock()
			tx.ds = nil
			return nil
		},
	}
}

func (q *Queue[Data]) Subscribe(ctx context.Context) pubsub.Subscription[Data] {
	// createdAt is set before returning the closure so that any publish that
	// races with Subscribe (i.e. happens after Subscribe returns but before
	// the goroutine starts iterating the subscription) is correctly included
	// under Volatile semantics. If we set createdAt inside the closure body,
	// such a publish would have qm.timestamp < s.createdAt and be filtered out.
	createdAt := clock.Now()
	return func(yield func(pubsub.Message[Data], error) bool) {
		sub := &QueueSubscription[Data]{
			ctx:       ctx,
			q:         q,
			createdAt: createdAt,
		}
		for i := 1; i < math.MaxInt; i++ {
			q.m.Lock()
			sub.id = subscriptionID(i)
			if q.subs == nil {
				q.subs = make(map[subscriptionID]*QueueSubscription[Data])
			}
			if _, ok := q.subs[sub.id]; !ok {
				q.subs[sub.id] = sub
				q.m.Unlock()
				break
			}
			q.m.Unlock()
		}
		defer sub.Close()
		for sub.Next() {
			v := sub.Value()
			if !yield(v, nil) {
				return
			}
		}
		var zero pubsub.Message[Data]
		if err := sub.Err(); err != nil {
			if !yield(zero, err) {
				return
			}
		}
		if err := sub.Close(); err != nil {
			if !yield(zero, err) {
				return
			}
		}
	}
}

const errBlockingQueueCannotBeUsedInTransaction errorkit.Error = "A blocking queue can't publish message as part of a transaction, as it would lead to a DEAD LOCK"

// BeginTx creates a context with a transaction.
// All statements that receive this context should be executed within the given transaction in the context.
// After a BeginTx command will be executed in a single transaction until an explicit COMMIT or ROLLBACK is given.
//
// In case the resource support some form of isolation level,
// or other ACID related property of the transaction,
// then it is advised to prepare this information in the context before calling BeginTx.
// e.g.:
//
//	...
//	var err error
//	ctx = r.ContextWithIsolationLevel(ctx, sql.LevelSerializable)
//	ctx, err = r.BeginTx(ctx)
func (q *Queue[Data]) BeginTx(ctx context.Context) (context.Context, error) {
	if q.Blocking {
		return nil, errBlockingQueueCannotBeUsedInTransaction
	}
	return q.txm().BeginTx(ctx)
}

// CommitTx Commit commits the current transaction.
// All changes made by the transaction become visible to others and are guaranteed to be durable if a crash occurs.
func (q *Queue[Data]) CommitTx(ctx context.Context) error {
	return q.txm().CommitTx(ctx)
}

// RollbackTx rolls back the current transaction and causes all the updates made by the transaction to be discarded.
func (q *Queue[Data]) RollbackTx(ctx context.Context) error {
	return q.txm().RollbackTx(ctx)
}

func (q *Queue[Data]) Purge(ctx context.Context) error {
	q.m.Lock()
	defer q.m.Unlock()

	// TODO: add trasnaction

	q.msgs = nil
	return nil
}

type QueueSubscription[Data any] struct {
	ctx context.Context
	q   *Queue[Data]
	id  subscriptionID

	closed    bool
	createdAt time.Time

	value *pubsubMessage[Data]
	err   error
}

func (pss *QueueSubscription[Data]) Close() error {
	if pss.closed {
		return nil
	}
	pss.closed = true
	if pss.value != nil {
		pss.value.NACK()
		pss.value = nil
	}
	return nil
}

func (s *QueueSubscription[Data]) Err() error {
	return s.err
}

func (s *QueueSubscription[Data]) Next() bool {
	if s.err != nil {
		return false
	}

	if err := s.ctx.Err(); err != nil {
		return false
	}

	if s.value != nil {
		s.value.NACK()
		s.value = nil
	}

	if s.closed {
		return false
	}

	msg, ctx, ack, nack, err := s.q.take(s.ctx, s)
	if err != nil {
		s.err = err
		return false
	}

	s.value = &pubsubMessage[Data]{
		ctx:  ctx,
		q:    s.q,
		sub:  s,
		msg:  msg,
		ack:  ack,
		nack: nack,
	}

	return true
}

func (s *QueueSubscription[Data]) Value() pubsub.Message[Data] {
	if s.value == nil {
		return pubsub.ZeroMessage[Data]()
	}
	return s.value
}

type pubsubMessage[Data any] struct {
	ctx context.Context

	q   *Queue[Data]
	sub *QueueSubscription[Data]
	msg *queueMessage[Data]

	ack  func() error
	nack func() error
}

func (pm *pubsubMessage[Data]) Context() context.Context {
	return pm.ctx
}

func (pm *pubsubMessage[Data]) ACK() error {
	if pm.msg == nil {
		return fmt.Errorf(".Value accessed before iter.Next, nothing to ACK")
	}
	return pm.ack()
}

func (pm *pubsubMessage[Data]) NACK() error {
	if pm.msg == nil {
		return fmt.Errorf(".Value accessed before iter.Next, nothing to NACK")
	}
	return pm.nack()
}

func (pm *pubsubMessage[Data]) Data() Data {
	if pm.msg == nil {
		return *new(Data)
	}
	return pm.msg.v
}

type queueMessage[Data any] struct {
	q *Queue[Data]
	v Data

	id string

	timestamp time.Time
	takenBy   subscriptionID
}

type subscriptionID int32

func (msg *queueMessage[Data]) take(subID subscriptionID) bool {
	for {
		curTakenBy := atomic.LoadInt32((*int32)(&msg.takenBy))
		if curTakenBy != 0 {
			return false
		}
		if curTakenBy == (int32)(subID) {
			return true
		}
		if atomic.CompareAndSwapInt32((*int32)(&msg.takenBy), 0, (int32)(subID)) {
			return true
		}
	}
}

// Release means the message is NACK -ed and should be picked up again.
func (rec *queueMessage[Data]) release(subID subscriptionID) {
	for {
		curTakenBy := atomic.LoadInt32((*int32)(&rec.takenBy))
		if curTakenBy == 0 { // already released
			return
		}
		if curTakenBy != (int32)(subID) {
			return // impossible to release
		}
		if atomic.CompareAndSwapInt32((*int32)(&rec.takenBy), (int32)(subID), 0) {
			break
		}
	}
}

//--------------------------------------------------------------------------------------------------------------------//

// FanOutExchange delivers messages to all the queues that are bound to it.
// This is useful when you want to broadcast a message to multiple consumers.
type FanOutExchange[Data any] struct {
	mx sync.RWMutex
	qs map[uint]*Queue[Data]
}

// Publish will publish all data to all FanOutExchange.Queues in an atomic fashion.
// It will either all succeed or all fail together.
func (e *FanOutExchange[Data]) Publish(ctx context.Context, data Data) (rErr error) {
	return e.eachQueue(ctx, func(ctx context.Context, q *Queue[Data]) error {
		return q.Publish(ctx, data)
	})
}

// Purge will flush all data from the exchange's queues
func (e *FanOutExchange[Data]) Purge(ctx context.Context) (rErr error) {
	return e.eachQueue(ctx, func(ctx context.Context, q *Queue[Data]) error {
		return q.Purge(ctx)
	})
}

func (e *FanOutExchange[Data]) eachQueue(ctx context.Context, blk func(ctx context.Context, q *Queue[Data]) error) (rErr error) {
	for _, q := range e.queues() {
		tx, err := q.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer comproto.FinishOnePhaseCommit(&rErr, q, tx)
		if err := blk(tx, q); err != nil {
			return err
		}
	}
	return nil
}

// queues is the list of queues currently bound to the exchange.
//
// The list is copied on purpose. A fan out operation must see one stable set of
// queues from beginning to end, while binding and unbinding is free to change
// the exchange's own registry at any moment from another goroutine.
func (e *FanOutExchange[Data]) queues() []*Queue[Data] {
	e.mx.RLock()
	defer e.mx.RUnlock()
	return mapkit.Values(e.qs)
}

var _ ds.Len = (*FanOutExchange[any])(nil)

func (e *FanOutExchange[Data]) Len() int {
	e.mx.RLock()
	defer e.mx.RUnlock()
	return len(e.qs)
}

// MakeQueue creates a unique queue which is bound to the FanOut exchange.
//
// The queue stays bound for as long as the exchange lives, so it collects every
// broadcast from the moment it was made, even while nothing consumes it.
func (e *FanOutExchange[Data]) MakeQueue() *Queue[Data] {
	q, _ := e.bindQueue()
	return q
}

var _ pubsub.Subscriber[string] = (*FanOutExchange[string])(nil)

// Subscribe consumes the exchange through a queue that belongs to this
// subscription alone, so every subscription receives every broadcast message.
//
// The queue lives only as long as the subscription does. Messages broadcast
// while no one is subscribed are dropped instead of being kept for a later
// subscriber; use FanOutExchange.MakeQueue when they must be retained.
func (e *FanOutExchange[Data]) Subscribe(ctx context.Context) pubsub.Subscription[Data] {
	// The queue is bound here instead of in the sequence body, so that a publish
	// racing with Subscribe is still delivered. Consuming a subscription
	// typically happens on another goroutine, and binding there would silently
	// drop everything broadcast until that goroutine gets scheduled.
	q, unbind := e.bindQueue()

	var once sync.Once
	unsubscribe := func() { once.Do(unbind) }

	// A subscription that is never consumed can only be released through its
	// context, and until then its queue keeps collecting messages that no one
	// will ever read.
	stopWatchingCtx := context.AfterFunc(ctx, unsubscribe)

	return func(yield func(pubsub.Message[Data], error) bool) {
		defer unsubscribe()
		defer stopWatchingCtx()
		for msg, err := range q.Subscribe(ctx) {
			if !yield(msg, err) {
				return
			}
		}
	}
}

// bindQueue creates a queue that receives what the exchange broadcasts,
// along with the func that unbinds it again.
func (e *FanOutExchange[Data]) bindQueue() (*Queue[Data], func()) {
	e.mx.Lock()
	defer e.mx.Unlock()

	if e.qs == nil {
		e.qs = map[uint]*Queue[Data]{}
	}

	var (
		q  = &Queue[Data]{}
		id = mapkit.FreeKey(e.qs, uint(len(e.qs)), e.nextID)
	)

	e.qs[id] = q

	return q, func() {
		e.mx.Lock()
		defer e.mx.Unlock()
		delete(e.qs, id)
	}
}

func (e *FanOutExchange[Data]) nextID(id uint) uint { return id + 1 }

//--------------------------------------------------------------------------------------------------------------------//
