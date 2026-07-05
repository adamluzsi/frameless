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

	_n    int
	_ch   chan T
	_done chan struct{}
}

var _ pubsub.Publisher[struct{}] = (*Fan[struct{}])(nil)

func (ps *Fan[T]) Publish(ctx context.Context, vs ...T) error {
	var ch, dn = ps.init()
	for _, v := range vs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-dn:
			return context.Canceled
		case ch <- v:
		}
	}
	return nil
}

var _ pubsub.Subscriber[struct{}] = (*Fan[struct{}])(nil)

func (ps *Fan[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] {
	return func(yield func(pubsub.Message[T], error) bool) {
		atomic.AddInt64(&ps.len, 1)
		defer atomic.AddInt64(&ps.len, -1)
		var ch, dn = ps.init()
		for {
			select {
			case <-ctx.Done():
				return
			case <-dn:
				return
			case v, ok := <-ch:
				if !ok {
					return
				}
				var msg = fanMessage[T]{
					ctx:  ctx,
					data: v,
					ack: func() error {
						return nil
					},
					nack: func() error {
						if ps.Len() == 1 {
							return fmt.Errorf("unable to NACK message, no other subscribers")
						}
						return ps.Publish(ctx, v)
					},
				}
				var cont = yield(&msg, nil)
				msg.done.Do(func() {
					msg.nack()
				})
				if !cont {
					return
				}
			}
		}
	}
}

var _ io.Closer = (*Fan[struct{}])(nil)

func (ps *Fan[T]) Close() error {
	var _, done = ps.init()
	select {
	case <-done:
	default:
		close(done)
	}
	return nil
}

var _ ds.Len = (*Fan[any])(nil)

func (ps *Fan[T]) Len() int {
	return int(atomic.LoadInt64(&ps.len))
}

func (ps *Fan[T]) init() (chan T, chan struct{}) {
	ps.o.Do(func() {
		ps._done = make(chan struct{})
		ps._ch = make(chan T)
	})
	return ps._ch, ps._done
}

type fanMessage[T any] struct {
	ctx  context.Context
	ack  func() error
	nack func() error
	data T

	done    sync.Once
	ackErr  error
	nackErr error
}

func (msg *fanMessage[T]) Context() context.Context {
	return msg.ctx
}

func (msg *fanMessage[T]) ACK() error {
	msg.done.Do(func() {
		msg.ackErr = msg.ack()
	})
	return msg.ackErr
}

func (msg *fanMessage[T]) NACK() error {
	msg.done.Do(func() {
		msg.nackErr = msg.nack()
	})
	return msg.nackErr
}

func (msg *fanMessage[T]) Data() T {
	return msg.data
}
