package frameless

import (
	"context"
	"io"
)

type Publisher /* [T] */ interface {
	// Subscribe create a subscription that will consume an event feed.
	// If event stream repeatability from a certain point is a requirement,
	// it needs to be further specified with a resource contract.
	Subscribe(context.Context, Subscriber) (Subscription, error)
}

type Subscriber /* T */ interface {
	// Handle handles the the subscribed event.
	// Context may or may not have meta information about the received event.
	// To ensure expectations, define a resource specification <contract> about what must be included in the context.
	Handle(ctx context.Context, event /* [T] */ interface{}) error
	// Error allow the subscription implementation to be notified about unexpected situations
	// that needs to be handled by the subscriber.
	// For e.g. the connection is lost and the subscriber might have cached values
	// that must be invalidated on the next successful Handle call
	Error(ctx context.Context, err error) error
}

type Subscription interface {
	io.Closer
}

type (
	EventCreate /*[T]*/ struct{ Entity T }
	EventUpdate /*[T]*/ struct{ Entity T }
	EventDeleteByID     struct{ ID ID }
	EventDeleteAll      struct{}
)
