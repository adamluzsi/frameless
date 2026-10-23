package synckit

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/pubsub"
)

type Fan[T any] struct {
	exchangeBase[T]
	channel chan T
	// backlog is shared between the subscribers,
	// as any of them can take over a NACK-ed message.
	backlog backlog[T]

	o sync.Once
}

var _ pubsub.Publisher[struct{}] = (*Fan[struct{}])(nil)
var _ pubsub.Subscriber[struct{}] = (*Fan[struct{}])(nil)
var _ ds.Len = (*Fan[any])(nil)
var _ io.Closer = (*Fan[struct{}])(nil)

func (fan *Fan[T]) init() {
	fan.o.Do(func() {
		fan.channel = make(chan T)
		fan.exchangeBase.init()
	})
}

func (fan *Fan[T]) Publish(ctx context.Context, v T) error {
	fan.init()
	return fan.exchangeBase.publish(ctx, fan.channel, nil, &fan.backlog, v)
}

func (fan *Fan[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] {
	fan.init()
	return fan.exchangeBase.subscribe(ctx, fan.channel, &fan.backlog)
}

// --------------------------

type Broadcast[T any] struct {
	exchangeBase[T]

	m sync.RWMutex
	s map[int]broadcastSubscriber[T]
	// orphans are the NACK-ed messages of the subscribers which left before they got them redelivered.
	// A later subscription takes over the orphans of a subscriber, like the subscriber would have re-subscribed.
	orphans [][]T
}

// broadcastSubscriber is the receiving end of a Broadcast subscription.
type broadcastSubscriber[T any] struct {
	ch chan T
	// left is closed when the subscriber leaves its subscription,
	// so a publish no longer waits to hand a message over to it.
	left chan struct{}
	// backlog is owned by the subscriber, as every subscriber receives its own copy of a message,
	// thus a NACK-ed message is only redelivered to the subscriber which NACK-ed it.
	backlog *backlog[T]
}

var _ pubsub.Publisher[string] = (*Broadcast[string])(nil)

func (b *Broadcast[T]) Publish(ctx context.Context, data T) error {
	// The lock is not held while waiting on the subscribers,
	// so subscribers can join or leave while a publish waits on a busy one.
	for _, sub := range b.subscribers() {
		if err := b.exchangeBase.publish(ctx, sub.ch, sub.left, sub.backlog, data); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broadcast[T]) subscribers() []broadcastSubscriber[T] {
	b.m.RLock()
	defer b.m.RUnlock()
	var subs = make([]broadcastSubscriber[T], 0, len(b.s))
	for _, sub := range b.s {
		subs = append(subs, sub)
	}
	return subs
}

var _ pubsub.Subscriber[string] = (*Broadcast[string])(nil)

func (b *Broadcast[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] {
	return func(yield func(pubsub.Message[T], error) bool) {
		// sub.ch is never closed, as a publish may still try to send on it after the subscriber left.
		var sub = broadcastSubscriber[T]{
			ch:      make(chan T),
			left:    make(chan struct{}),
			backlog: &backlog[T]{pending: b.adoptOrphans()},
		}
		defer register(&b.m, &b.s, sub)()
		defer b.abandon(sub.backlog)
		defer close(sub.left)
		for v, err := range b.exchangeBase.subscribe(ctx, sub.ch, sub.backlog) {
			if !yield(v, err) {
				return
			}
		}
	}
}

func (b *Broadcast[T]) adoptOrphans() []T {
	b.m.Lock()
	defer b.m.Unlock()
	orphans, _ := slicekit.Shift(&b.orphans)
	return orphans
}

// abandon leaves the NACK-ed messages of a departing subscriber to a later subscription.
func (b *Broadcast[T]) abandon(bl *backlog[T]) {
	var pending = bl.drain()
	if len(pending) == 0 {
		return
	}
	b.m.Lock()
	defer b.m.Unlock()
	b.orphans = append(b.orphans, pending)
}

// ---

type exchangeBase[T any] struct {
	o   sync.Once
	len int64

	mutex   sync.RWMutex
	cancels map[int]func()
	closed  chan struct{}
}

// backlog keeps track of the messages which still may require processing:
// the ones handed over to a subscriber but not yet settled (in-flight),
// and the NACK-ed ones which wait for their redelivery (pending).
type backlog[T any] struct {
	mutex    sync.Mutex
	pending  []T
	inflight int
	// changed is closed on the next change of the backlog.
	// It is nil while no one waits on a change.
	changed chan struct{}
}

// handOver marks a message as in-flight before it is handed over to a subscriber,
// so the message is accounted for while it is in transit between a publisher and a subscriber.
func (bl *backlog[T]) handOver() {
	bl.mutex.Lock()
	defer bl.mutex.Unlock()
	bl.inflight++
}

// settle marks an in-flight message as no longer requiring processing,
// either because it was ACK-ed or dropped, or because it could not be handed over.
func (bl *backlog[T]) settle() {
	bl.mutex.Lock()
	defer bl.mutex.Unlock()
	bl.inflight--
	bl.notify()
}

// putBack makes an in-flight message pending for redelivery.
func (bl *backlog[T]) putBack(v T) {
	bl.mutex.Lock()
	defer bl.mutex.Unlock()
	bl.inflight--
	bl.pending = append(bl.pending, v)
	bl.notify()
}

// take a pending message for redelivery, which makes it in-flight again.
func (bl *backlog[T]) take() (T, bool) {
	bl.mutex.Lock()
	defer bl.mutex.Unlock()
	v, ok := slicekit.Shift(&bl.pending)
	if ok {
		bl.inflight++
	}
	return v, ok
}

func (bl *backlog[T]) drain() []T {
	bl.mutex.Lock()
	defer bl.mutex.Unlock()
	var pending = bl.pending
	bl.pending = nil
	return pending
}

// settled tells whether nothing requires processing anymore.
// While messages are in-flight, but none of them is pending for redelivery,
// it returns a channel which is closed on the next change of the backlog.
func (bl *backlog[T]) settled() (bool, <-chan struct{}) {
	bl.mutex.Lock()
	defer bl.mutex.Unlock()
	if 0 < len(bl.pending) {
		return false, nil
	}
	if bl.inflight == 0 {
		return true, nil
	}
	if bl.changed == nil {
		bl.changed = make(chan struct{})
	}
	return false, bl.changed
}

func (bl *backlog[T]) notify() {
	if bl.changed != nil {
		close(bl.changed)
		bl.changed = nil
	}
}

func (ex *exchangeBase[T]) init() {
	ex.o.Do(func() {
		ex.closed = make(chan struct{})
		ex.cancels = make(map[int]func())
	})
}

// publish hands data over to the receiver of ch.
// When the receiver signals through left that it is gone, there is no one to hand data over to.
// A nil left means the receiver never leaves.
func (ex *exchangeBase[T]) publish(ctx context.Context, ch chan<- T, left <-chan struct{}, bl *backlog[T], data T) error {
	ex.init()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	bl.handOver()
	select {
	case <-ctx.Done():
		bl.settle()
		return ctx.Err()
	case <-ex.closed:
		bl.settle()
		return context.Canceled
	case <-left:
		bl.settle()
	case ch <- data:
	}
	return nil
}

var _ pubsub.Subscriber[struct{}] = (*Fan[struct{}])(nil)

func (ex *exchangeBase[T]) subscribe(ctx context.Context, ch chan T, bl *backlog[T]) pubsub.Subscription[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	return func(yield func(pubsub.Message[T], error) bool) {
		ex.init()
		atomic.AddInt64(&ex.len, 1)
		defer atomic.AddInt64(&ex.len, -1)
		var (
			ack = func(pubsub.Message[T]) error {
				bl.settle()
				return nil
			}
			nack = func(msg pubsub.Message[T]) error {
				select {
				case <-msg.Context().Done():
					bl.settle()
					return msg.Context().Err()
				case ch <- msg.Data(): // handed over to an idle subscriber, so it stays in-flight
					return nil
				default:
					bl.putBack(msg.Data())
					return nil
				}
			}
		)
		var handle = func(ctx context.Context, data T) bool {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			defer ex.regCancel(cancel)()
			var msg = pubsub.MakeMessage(ctx, data, ack, nack)
			defer msg.NACK()
			return yield(msg, nil)
		}
		for {
			if v, ok := bl.take(); ok {
				if !handle(ctx, v) {
					return
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case v := <-ch:
				if !handle(ctx, v) {
					return
				}
			case <-ex.closed:
				// Close only ends the subscription once nothing requires processing,
				// as an in-flight message may still get NACK-ed back for redelivery.
				settled, changed := bl.settled()
				if settled {
					return
				}
				if changed == nil { // a message is pending for redelivery
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-changed:
				case v := <-ch:
					if !handle(ctx, v) {
						return
					}
				}
			}
		}
	}
}

func register[T any](l sync.Locker, register *map[int]T, v T) func() {
	l.Lock()
	defer l.Unlock()
	var id int
	if *register == nil {
		*register = make(map[int]T)
	}
	for i := len(*register); ; i++ {
		if _, ok := (*register)[i]; !ok {
			id = i
			break
		}
	}
	(*register)[id] = v
	return func() {
		l.Lock()
		defer l.Unlock()
		delete(*register, id)
	}
}

func (ex *exchangeBase[T]) regCancel(cancel func()) func() {
	return register(&ex.mutex, &ex.cancels, cancel)
}

var _ ds.Len = (*exchangeBase[any])(nil)

func (ex *exchangeBase[T]) Len() int {
	return int(atomic.LoadInt64(&ex.len))
}

func (ex *exchangeBase[T]) Close() error {
	ex.init()
	ex.mutex.Lock()
	defer ex.mutex.Unlock()
	select {
	case <-ex.closed:
	default:
		close(ex.closed)
	}
	return nil
}

func (ex *exchangeBase[T]) Cancel() {
	ex.init()
	_ = ex.Close()
	ex.mutex.Lock()
	defer ex.mutex.Unlock()
	for _, cancel := range ex.cancels {
		cancel()
	}
}
