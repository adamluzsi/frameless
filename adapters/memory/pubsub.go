package memory

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/testcase/clock"
	"github.com/adamluzsi/testcase/random"
	"sort"
	"sync/atomic"
	"time"
)

type Queue[Entity any] struct {
	Memory *Memory
	// Namespace allows you to isolate two different Queue while using the same *Memory
	Namespace string
	// LIFO is a flag to change element ordering from FIFO to LIFO
	LIFO bool
	// Volatile will flag the Queue to act like a Volatile queue
	Volatile bool
	// blocking will cause the Queue to wait until the published messages are ACK -ed.
	Blocking bool
}

const typeNameQueue = "Queue"

func (ps *Queue[Entity]) Publish(ctx context.Context, vs ...Entity) (rErr error) {
	var (
		keys      []string
		namespace = getNamespaceFor[Entity](typeNameQueue, &ps.Namespace)
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
			ps.Memory.Set(ctx, namespace, key, &pubsubRecord[Entity]{
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
			if _, ok := memoryLookup[*pubsubRecord[Entity]](ps.Memory, namespace, key); ok {
				goto check
			}
		}
	}

	return nil
}

func (ps *Queue[Entity]) Subscribe(ctx context.Context) iterators.Iterator[pubsub.Message[Entity]] {
	return &pubsubSubscription[Entity]{
		ctx:       ctx,
		pubsub:    ps,
		createdAt: clock.TimeNow(),
		closed:    false,
	}
}

func (ps *Queue[Entity]) Purge(ctx context.Context) error {
	var namespace = getNamespaceFor[Entity](typeNameQueue, &ps.Namespace)
	for key, _ := range ps.Memory.all(namespace) {
		ps.Memory.Del(ctx, namespace, key)
	}
	return nil
}

type pubsubRecord[Entity any] struct {
	key       string
	value     Entity
	createdAt time.Time
	taken     int32
}

func (rec *pubsubRecord[Entity]) Take() bool {
	return atomic.CompareAndSwapInt32(&rec.taken, 0, 1)
}

func (rec *pubsubRecord[Entity]) Release() {
	atomic.CompareAndSwapInt32(&rec.taken, 1, 0)
}

func (ps *Queue[Entity]) makeKey() string {
	return random.New(random.CryptoSeed{}).UUID()
}

type pubsubSubscription[Entity any] struct {
	ctx    context.Context
	pubsub *Queue[Entity]

	closed    bool
	createdAt time.Time

	value *pubsubRecord[Entity]
	err   error
}

func (pss *pubsubSubscription[Entity]) Close() error {
	pss.closed = true
	if pss.value != nil {
		pss.value.Release()
		pss.value = nil
	}
	return nil
}

func (pss *pubsubSubscription[Entity]) Err() error {
	return pss.err
}

func (pss *pubsubSubscription[Entity]) Next() bool {
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

	namespace := getNamespaceFor[Entity](typeNameQueue, &pss.pubsub.Namespace)
	iter := memoryAll[*pubsubRecord[Entity]](pss.pubsub.Memory, pss.ctx, namespace)
	iter = iterators.Filter(iter, func(r *pubsubRecord[Entity]) bool {
		if pss.pubsub.Volatile {
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
	sort.Slice(recs, func(i, j int) bool {
		less := recs[i].createdAt.Before(recs[j].createdAt)
		if pss.pubsub.LIFO {
			return !less
		}
		return less
	})

	var record *pubsubRecord[Entity]
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

func (pss *pubsubSubscription[Entity]) Value() pubsub.Message[Entity] {
	return &pubsubMessage[Entity]{
		ctx:    pss.ctx,
		pubsub: pss.pubsub,
		record: pss.value,
	}
}

type pubsubMessage[Entity any] struct {
	ctx    context.Context
	pubsub *Queue[Entity]
	record *pubsubRecord[Entity]
}

func (pm *pubsubMessage[Entity]) ACK() error {
	if pm.record == nil {
		return fmt.Errorf(".Value accessed before iter.Next, nothing to ACK")
	}
	_, ok := pm.pubsub.Memory.lookup(getNamespaceFor[Entity](typeNameQueue, &pm.pubsub.Namespace), pm.record.key)
	if !ok {
		return nil
	}
	pm.pubsub.Memory.Del(pm.ctx, getNamespaceFor[Entity](typeNameQueue, &pm.pubsub.Namespace), pm.record.key)
	return nil
}

func (pm *pubsubMessage[Entity]) NACK() error {
	if pm.record == nil {
		return fmt.Errorf(".Value accessed before iter.Next, nothing to NACK")
	}
	pm.record.Release()
	return nil
}

func (pm *pubsubMessage[Entity]) Data() Entity {
	if pm.record == nil {
		return *new(Entity)
	}
	return pm.record.value
}
