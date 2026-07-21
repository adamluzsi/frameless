package synckit

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"go.llib.dev/frameless/port/ds"
	"go.llib.dev/frameless/port/pubsub"
)

type Fan[T any] struct {
	o   sync.Once
	len int64

	mutex   sync.Mutex
	cancels map[int]func()
	channel chan T
	closed  chan struct{}
}

func (fan *Fan[T]) init() {
	fan.o.Do(func() {
		fan.channel = make(chan T)
		fan.closed = make(chan struct{})
		fan.cancels = make(map[int]func())
	})
}

var _ pubsub.Publisher[struct{}] = (*Fan[struct{}])(nil)

func (fan *Fan[T]) Publish(ctx context.Context, v T) error {
	fan.init()
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-fan.closed:
		return context.Canceled
	case fan.channel <- v:
	}
	return nil
}

var _ pubsub.Subscriber[struct{}] = (*Fan[struct{}])(nil)

func (fan *Fan[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] {
	return func(yield func(pubsub.Message[T], error) bool) {
		fan.init()
		atomic.AddInt64(&fan.len, 1)
		defer atomic.AddInt64(&fan.len, -1)
		for {
			select {
			case <-ctx.Done():
				return
			case <-fan.closed:
				return
			case v, ok := <-fan.channel:
				if !ok {
					return
				}
				if !fan.handle(ctx, v, yield) {
					return
				}
			}
		}
	}
}
func (fan *Fan[T]) handle(ctx context.Context, v T, yield func(pubsub.Message[T], error) bool) bool {
	msgCtx, cancel := context.WithCancel(ctx)
	defer fan.regCancel(cancel)()
	var msg = fanMessage[T]{
		fan:  fan,
		ctx:  msgCtx,
		data: v,
	}
	defer msg.finish()
	var cont = yield(&msg, nil)
	return cont
}

var _ io.Closer = (*Fan[struct{}])(nil)

func (fan *Fan[T]) Close() error {
	fan.init()
	select {
	case <-fan.closed:
	default:
		close(fan.closed)
	}
	return nil
}

func (fan *Fan[T]) Cancel() {
	fan.init()
	_ = fan.Close()
	fan.mutex.Lock()
	defer fan.mutex.Unlock()
	for _, cancel := range fan.cancels {
		cancel()
	}
}

func (fan *Fan[T]) regCancel(cancel func()) func() {
	fan.mutex.Lock()
	defer fan.mutex.Unlock()
	var id int
	for i := len(fan.cancels); ; i++ {
		if _, ok := fan.cancels[i]; !ok {
			id = i
			break
		}
	}
	fan.cancels[id] = cancel
	return func() {
		fan.mutex.Lock()
		defer fan.mutex.Unlock()
		delete(fan.cancels, id)
	}
}

var _ ds.Len = (*Fan[any])(nil)

func (fan *Fan[T]) Len() int {
	return int(atomic.LoadInt64(&fan.len))
}

type fanMessage[T any] struct {
	fan *Fan[T]

	ctx  context.Context
	ack  func() error
	nack func() error
	data T

	done   sync.Once
	ackErr error
	err    error
}

func (msg *fanMessage[T]) Context() context.Context {
	return msg.ctx
}

func (msg *fanMessage[T]) Data() T {
	return msg.data
}

func (msg *fanMessage[T]) ACK() error {
	msg.done.Do(func() {})
	return msg.err
}

func (msg *fanMessage[T]) NACK() error {
	msg.done.Do(func() {
		if msg.fan.Len() <= 1 {
			msg.err = fmt.Errorf("unable to NACK message, no other subscribers")
			return
		}
		msg.err = msg.fan.Publish(msg.ctx, msg.data)
	})
	return msg.err
}

func (msg *fanMessage[T]) finish() {
	// NACK won't run if msg.done is already triggered, aka, already ACK -ed
	_ = msg.NACK()
}
