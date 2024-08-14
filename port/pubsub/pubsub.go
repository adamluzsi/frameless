package pubsub

import (
	"context"

	"go.llib.dev/frameless/port/iterators"
)

type Publisher[Data any] interface {
	Publish(context.Context, ...Data) error
}

type Subscriber[Data any] interface {
	Subscribe(context.Context) Subscription[Data]
}

type Subscription[Data any] interface {
	iterators.Iterator[Message[Data]]
}

type Message[Data any] interface {
	ACK() error
	NACK() error
	Data() Data
}
