package synckit

import (
	"context"
	"sync"

	"go.llib.dev/frameless/port/pubsub"
)

type Fan[T any] struct {
	_ch chan T
	rwm sync.RWMutex
}

func (ps *Fan[T]) stream() chan T {
	return Init(&ps.rwm, &ps._ch, func() chan T {
		return make(chan T)
	})
}

var _ pubsub.Publisher[struct{}] = (*Fan[struct{}])(nil)

func (ps *Fan[T]) Publish(ctx context.Context, vs ...T) error {
	for _, v := range vs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ps.stream() <- v:
		}
	}
	return nil
}

var _ pubsub.Subscriber[struct{}] = (*Fan[struct{}])(nil)

func (ps *Fan[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] {
	return func(yield func(pubsub.Message[T], error) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-ps.stream():
				if !ok {
					return
				}
				var msg = messageFanOut[T]{
					ctx:  ctx,
					data: v,
					ack: func() error {
						return nil
					},
					nack: func() error {
						select {
						case ps.stream() <- v:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					},
				}
				if !yield(&msg, nil) {
					msg.done.Do(func() {
						msg.nack()
					})
					return
				}
			}
		}
	}
}

type messageFanOut[T any] struct {
	ctx  context.Context
	ack  func() error
	nack func() error
	data T

	done    sync.Once
	ackErr  error
	nackErr error
}

func (msg *messageFanOut[T]) Context() context.Context {
	return msg.ctx
}

func (msg *messageFanOut[T]) ACK() error {
	msg.done.Do(func() {
		msg.ackErr = msg.ack()
	})
	return msg.ackErr
}

func (msg *messageFanOut[T]) NACK() error {
	msg.done.Do(func() {
		msg.nackErr = msg.nack()
	})
	return msg.nackErr
}

func (msg *messageFanOut[T]) Data() T {
	return msg.data
}
