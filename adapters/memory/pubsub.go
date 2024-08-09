package memory

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/frameless/ports/pubsub"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/random"
)

type Queue[Data any] struct {
	Memory *Memory
	// Namespace allows you to isolate two different Queue while using the same *Memory
	Namespace string
	// LIFO is a flag to change element ordering from FIFO to LIFO
	LIFO bool
	// Volatile will flag the Queue to act like a Volatile queue
	Volatile bool
	// blocking will cause the Queue to wait until the published messages are ACK -ed.
	Blocking bool

	// SortLessFunc will define how to sort data, when we look for what message to handle next.
	// if not supplied FIFO is the default ordering.
	SortLessFunc func(i Data, j Data) bool
}

const typeNameQueue = "Queue"

func (ps *Queue[Data]) Publish(ctx context.Context, vs ...Data) (rErr error) {
	var (
		keys      []string
		namespace = getNamespaceFor[Data](typeNameQueue, &ps.Namespace)
	)
	if err := func(ctx context.Context) error {
		ctx, err := ps.Memory.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer comproto.FinishOnePhaseCommit(&rErr, ps.Memory, ctx)

		for _, v := range vs {
			key := ps.makeKey()
			keys = append(keys, key)
			ps.Memory.Set(ctx, namespace, key, &pubsubRecord[Data]{
				key:       key,
				value:     v,
				createdAt: time.Now().UTC(),
			})
		}
		return nil
	}(ctx); err != nil {
		return err
	}

	if ps.Blocking {
	check:
		for _, key := range keys {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, ok := memoryLookup[*pubsubRecord[Data]](ps.Memory, namespace, key); ok {
				goto check
			}
		}
	}

	return nil
}

func (ps *Queue[Data]) Subscribe(ctx context.Context) pubsub.Subscription[Data] {
	return &pubsubSubscription[Data]{
		ctx:       ctx,
		q:         ps,
		createdAt: clock.Now(),
		closed:    false,
	}
}

func (ps *Queue[Data]) Purge(ctx context.Context) error {
	var namespace = getNamespaceFor[Data](typeNameQueue, &ps.Namespace)
	for key, _ := range ps.Memory.all(namespace) {
		ps.Memory.Del(ctx, namespace, key)
	}
	return nil
}

type pubsubRecord[Data any] struct {
	key       string
	value     Data
	createdAt time.Time
	taken     int32
}

func (rec *pubsubRecord[Data]) Take() bool {
	return atomic.CompareAndSwapInt32(&rec.taken, 0, 1)
}

func (rec *pubsubRecord[Data]) Release() {
	atomic.CompareAndSwapInt32(&rec.taken, 1, 0)
}

func (ps *Queue[Data]) makeKey() string {
	return random.New(random.CryptoSeed{}).UUID()
}

type pubsubSubscription[Data any] struct {
	ctx context.Context
	q   *Queue[Data]

	closed    bool
	createdAt time.Time

	value *pubsubRecord[Data]
	err   error
}

func (pss *pubsubSubscription[Data]) Close() error {
	pss.closed = true
	if pss.value != nil {
		pss.value.Release()
		pss.value = nil
	}
	return nil
}

func (pss *pubsubSubscription[Data]) Err() error {
	return pss.err
}

func (pss *pubsubSubscription[Data]) Next() bool {
fetch:

	if pss.err != nil {
		return false
	}

	if err := pss.ctx.Err(); err != nil {
		return false
	}

	if pss.value != nil {
		pss.value.Release()
		pss.value = nil
	}

	if pss.closed {
		return false
	}

	namespace := getNamespaceFor[Data](typeNameQueue, &pss.q.Namespace)
	iter := memoryAll[*pubsubRecord[Data]](pss.q.Memory, pss.ctx, namespace)
	iter = iterators.Filter(iter, func(r *pubsubRecord[Data]) bool {
		if pss.q.Volatile {
			return pss.createdAt.Before(r.createdAt)
		}
		return true
	})
	recs, err := iterators.Collect(iter)
	if err != nil {
		pss.err = err
		return false
	}
	if 0 == len(recs) {
		goto fetch
	}

	pss.sort(recs)

	var record *pubsubRecord[Data]
	for _, rec := range recs {
		if rec.Take() {
			record = rec
			break
		}
	}
	if record == nil {
		goto fetch
	}

	pss.value = record
	return true
}

func (pss *pubsubSubscription[Data]) sort(recs []*pubsubRecord[Data]) {
	sort.Slice(recs, func(i, j int) bool {
		if pss.q.SortLessFunc != nil {
			return pss.q.SortLessFunc(recs[i].value, recs[j].value)
		}
		less := recs[i].createdAt.Before(recs[j].createdAt)
		if pss.q.LIFO {
			return !less
		}
		return less
	})
}

func (pss *pubsubSubscription[Data]) Value() pubsub.Message[Data] {
	return &pubsubMessage[Data]{
		ctx:    pss.ctx,
		pubsub: pss.q,
		record: pss.value,
	}
}

type pubsubMessage[Data any] struct {
	ctx    context.Context
	pubsub *Queue[Data]
	record *pubsubRecord[Data]
}

func (pm *pubsubMessage[Data]) ACK() error {
	if pm.record == nil {
		return fmt.Errorf(".Value accessed before iter.Next, nothing to ACK")
	}
	_, ok := pm.pubsub.Memory.lookup(getNamespaceFor[Data](typeNameQueue, &pm.pubsub.Namespace), pm.record.key)
	if !ok {
		return nil
	}
	pm.pubsub.Memory.Del(pm.ctx, getNamespaceFor[Data](typeNameQueue, &pm.pubsub.Namespace), pm.record.key)
	return nil
}

func (pm *pubsubMessage[Data]) NACK() error {
	if pm.record == nil {
		return fmt.Errorf(".Value accessed before iter.Next, nothing to NACK")
	}
	pm.record.Release()
	return nil
}

func (pm *pubsubMessage[Data]) Data() Data {
	if pm.record == nil {
		return *new(Data)
	}
	return pm.record.value
}

//--------------------------------------------------------------------------------------------------------------------//

// FanOutExchange delivers messages to all the queues that are bound to it.
// This is useful when you want to broadcast a message to multiple consumers.
type FanOutExchange[Data any] struct {
	Memory *Memory
	// Namespace allows you to isolate two different FanOutExchange while using the same *Memory
	Namespace string
	// Queues contain every Queue that suppose to be bound to the FanOut Exchange
	Queues []*Queue[Data]
}

// Publish will publish all data to all FanOutExchange.Queues in an atomic fashion.
// It will either all succeed or all fail together.
func (e *FanOutExchange[Data]) Publish(ctx context.Context, data ...Data) (rErr error) {
	ctx, err := e.Memory.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, e.Memory, ctx)

	for _, q := range e.Queues {
		if err := q.Publish(ctx, data...); err != nil {
			return err
		}
	}
	return nil
}

// Purge will flush all data from the exchange's queues
func (e *FanOutExchange[Data]) Purge(ctx context.Context) (rErr error) {
	ctx, err := e.Memory.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, e.Memory, ctx)
	for _, q := range e.Queues {
		if err := q.Purge(ctx); err != nil {
			return err
		}
	}
	return nil
}

// MakeQueue creates a unique queue which is bound to the FanOut exchange.
func (e *FanOutExchange[Data]) MakeQueue() *Queue[Data] {
	q := &Queue[Data]{
		Memory:    e.Memory,
		Namespace: fmt.Sprintf("%s/queues/%s", e.getNamespace(), rnd.UUID()),
	}
	e.Queues = append(e.Queues, q)
	return q
}

func (e *FanOutExchange[Data]) getNamespace() string {
	return getNamespaceFor[Data]("Exchange", &e.Namespace)
}

//--------------------------------------------------------------------------------------------------------------------//
