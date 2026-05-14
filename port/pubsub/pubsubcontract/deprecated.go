package pubsubcontract

import (
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/pubsub"
)

// Buffered
//
// Deprecated: use pubsubcontract.Durable[Data] instead.
func Buffered[Data any](publisher pubsub.Publisher[Data], subscriber pubsub.Subscriber[Data], opts ...Option[Data]) contract.Contract {
	return Durable[Data](publisher, subscriber, opts...)
}
