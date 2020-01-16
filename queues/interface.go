package queues

import (
	"context"
)

type Queue interface {
	Publisher
	Subscriber
}

type Publisher interface {
	Publish(ctx context.Context, request Request) (committer Signaler, err error)
}

type Request interface {
	Values() []interface{}
	Meta() map[string][]string
}

type Subscriber interface {
	Get(ctx context.Context) (Response, error)
}

type Response interface {
	Value() interface{}
	Signaler
}

// Signaler represent communication type of interactions about message state handling.
type Signaler interface {
	// Acknowledgement represent a success confirmation back to a caller.
	//
	// For example, in data networking, telecommunications, and computer buses,
	// an acknowledgement (ACK) is a signal that is passed between communicating processes,
	// computers, or devices to signify acknowledgement, or receipt of message, as part of a communications protocol.
	Acknowledgement() error
	// NegativeAcknowledgement represent a signal that tells the caller that the received resource must be considered as not handled,
	// and should be handled over again eventually.
	//
	// The negative-acknowledgement (NAK or NACK) signal is sent to reject a previously received message,
	// or to indicate some kind of error.
	//
	// Acknowledgements and negative acknowledgements inform a sender of the receiver's state
	// so that it can adjust its own state accordingly.
	NegativeAcknowledgement() error
}
