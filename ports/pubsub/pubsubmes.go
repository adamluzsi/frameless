package pubsub

import (
	"context"
	"github.com/adamluzsi/frameless/ports/iterators"
)

type Publisher[V any] interface {
	Publish(context.Context, ...V) error
}

type Subscriber[V any] interface {
	Subscribe(context.Context) iterators.Iterator[Message[V]]
}

type Message[V any] interface {
	ACK() error
	NACK() error
	Data() V
}
