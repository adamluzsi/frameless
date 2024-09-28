package pubsub

import (
	"context"

	"go.llib.dev/frameless/port/iterators"
)

type Publisher[Data any] interface {
	Publish(context.Context, ...Data) error
}

type Subscriber[Data any] interface {
	Subscribe(context.Context) (Subscription[Data], error) // crud.QueryMany
}

type Subscription[Data any] interface {
	iterators.Iterator[Message[Data]]
}

type Message[Data any] interface {
	ACK() error
	NACK() error
	Data() Data
}

func ZeroMessage[Data any]() Message[Data] {
	return zeroMessage[Data]{}
}

type zeroMessage[Data any] struct{}

func (zeroMessage[Data]) ACK() error  { return nil }
func (zeroMessage[Data]) NACK() error { return nil }
func (zeroMessage[Data]) Data() Data  { return *new(Data) }
