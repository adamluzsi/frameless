package pubsub

import (
	"context"
	"iter"
	"sync"
)

type Publisher[Data any] interface {
	Publish(context.Context, Data) error
}

type Subscriber[Data any] interface {
	Subscribe(context.Context) Subscription[Data] // crud.QueryMany
}

type Subscription[Data any] = iter.Seq2[Message[Data], error]

type Message[Data any] interface {
	Context() context.Context
	Data() Data
	ACK() error
	NACK() error
}

func ZeroMessage[Data any]() Message[Data] {
	return zeroMessage[Data]{}
}

type zeroMessage[Data any] struct{}

func (zeroMessage[Data]) Context() context.Context { return context.Background() }
func (zeroMessage[Data]) Data() Data               { return *new(Data) }
func (zeroMessage[Data]) ACK() error               { return nil }
func (zeroMessage[Data]) NACK() error              { return nil }

func MakeMessage[Data any](ctx context.Context, data Data, ack, nack func(Message[Data]) error) Message[Data] {
	return &message[Data]{
		context: ctx,
		ack:     ack,
		nack:    nack,
		data:    data,
	}
}

type message[Data any] struct {
	context context.Context
	data    Data

	ack  func(Message[Data]) error
	nack func(Message[Data]) error

	fin sync.Once
	err error
}

var _ Message[string] = (*message[string])(nil)

func (msg *message[Data]) Context() context.Context {
	if msg.context != nil {
		return msg.context
	}
	return context.Background()
}

func (msg *message[Data]) Data() Data {
	return msg.data
}

func (msg *message[Data]) ACK() error {
	if msg.ack == nil {
		return nil
	}
	msg.fin.Do(func() {
		msg.err = msg.ack(msg)
	})
	return msg.err
}

func (msg *message[Data]) NACK() error {
	if msg.nack == nil {
		return nil
	}
	msg.fin.Do(func() {
		msg.err = msg.nack(msg)
	})
	return msg.err
}
