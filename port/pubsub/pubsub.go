package pubsub

import (
	"context"
	"iter"
)

type Publisher[Data any] interface {
	Publish(context.Context, ...Data) error
}

type Subscriber[Data any] interface {
	Subscribe(context.Context) (Subscription[Data], func() error, error) // crud.QueryMany
}

type Subscription[Data any] iter.Seq[Message[Data]]

type Message[Data any] interface {
	Context() context.Context
	ACK() error
	NACK() error
	Data() Data
}

func ZeroMessage[Data any]() Message[Data] {
	return zeroMessage[Data]{}
}

type zeroMessage[Data any] struct{}

func (zeroMessage[Data]) Context() context.Context { return context.Background() }
func (zeroMessage[Data]) ACK() error               { return nil }
func (zeroMessage[Data]) NACK() error              { return nil }
func (zeroMessage[Data]) Data() Data               { return *new(Data) }
