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
	return fan.exchangeBase.publish(ctx, fan.channel, v)
}

func (fan *Fan[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] {
	fan.init()
	return fan.exchangeBase.subscribe(ctx, fan.channel)
}

// --------------------------

type Broadcast[T any] struct {
	exchangeBase[T]

	m sync.RWMutex
	s map[int]chan T
}

var _ pubsub.Publisher[string] = (*Broadcast[string])(nil)

func (b *Broadcast[T]) Publish(ctx context.Context, data T) error {
	b.m.RLock()
	defer b.m.RUnlock()
	for _, ch := range b.s {
		if err := b.exchangeBase.publish(ctx, ch, data); err != nil {
			return err
		}
	}
	return nil
}

var _ pubsub.Subscriber[string] = (*Broadcast[string])(nil)

func (b *Broadcast[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] {
	return func(yield func(pubsub.Message[T], error) bool) {
		var ch = make(chan T)
		defer close(ch)
		defer register(&b.m, &b.s, ch)()
		for v, err := range b.exchangeBase.subscribe(ctx, ch) {
			if !yield(v, err) {
				return
			}
		}
	}
}

// ---

type exchangeBase[T any] struct {
	o   sync.Once
	len int64

	mutex   sync.RWMutex
	cancels map[int]func()
	closed  chan struct{}

	buffer []T
}

func (ex *exchangeBase[T]) bufferShift() (T, bool) {
	ex.mutex.RLock()
	if len(ex.buffer) == 0 {
		ex.mutex.RUnlock()
		var zero T
		return zero, false
	}
	ex.mutex.RUnlock()
	ex.mutex.Lock()
	defer ex.mutex.Unlock()
	return slicekit.Shift(&ex.buffer)
}

func (ex *exchangeBase[T]) init() {
	ex.o.Do(func() {
		ex.closed = make(chan struct{})
		ex.cancels = make(map[int]func())
	})
}

func (ex *exchangeBase[T]) publish(ctx context.Context, ch chan<- T, data T) error {
	ex.init()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ex.closed:
		return context.Canceled
	case ch <- data:
	}
	return nil
}

var _ pubsub.Subscriber[struct{}] = (*Fan[struct{}])(nil)

func (ex *exchangeBase[T]) subscribe(ctx context.Context, ch chan T) pubsub.Subscription[T] {
	if ctx == nil {
		ctx = context.Background()
	}
	return func(yield func(pubsub.Message[T], error) bool) {
		ex.init()
		atomic.AddInt64(&ex.len, 1)
		defer atomic.AddInt64(&ex.len, -1)
		var handle = func(ctx context.Context, data T) bool {
			ctx, cancel := context.WithCancel(ctx)
			defer ex.regCancel(cancel)()
			var msg = pubsub.MakeMessage(ctx, data, ex.ack, ex.defaultNack(ch))
			defer msg.NACK()
			return yield(msg, nil)
		}
		for {
			if first, ok := ex.bufferShift(); ok {
				if !handle(ctx, first) {
					return
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-ex.closed:
				return
			case v, ok := <-ch:
				if !ok {
					return
				}
				if !handle(ctx, v) {
					return
				}
			}
		}
	}
}

func (ex *exchangeBase[T]) ack(pubsub.Message[T]) error {
	return nil
}

func (ex *exchangeBase[T]) defaultNack(ch chan T) func(msg pubsub.Message[T]) error {
	return func(msg pubsub.Message[T]) error {
		select {
		case <-msg.Context().Done():
			return msg.Context().Err()
		case ch <- msg.Data():
			return nil
		default:
			ex.mutex.Lock()
			ex.buffer = append(ex.buffer, msg.Data())
			ex.mutex.Unlock()
			return nil
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
