package pubsub

import (
	"context"
	"github.com/adamluzsi/frameless/ports/iterators"
)

type Publisher[Data any] interface {
	Publish(context.Context, ...Data) error
}

type Subscriber[Data any] interface {
	Subscribe(context.Context) iterators.Iterator[Message[Data]]
}

type Message[Data any] interface {
	ACK() error
	NACK() error
	Data() Data
}
