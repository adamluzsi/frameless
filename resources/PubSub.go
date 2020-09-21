package resources

import (
	"context"
	"io"
)

type CreatorPublisher interface {
	// SubscribeToCreate create a subscription to create event feed.
	// 	eg.: storage.SubscribeToCreate(``, cache.CreateEventHandler())
	//
	// If event stream repeatability from a certain point is a requirement,
	// it needs to be further specified with a resource contract.
	SubscribeToCreate(context.Context, T, Subscriber) (Subscription, error)
}

type UpdaterPublisher interface {
	// SubscribeToUpdate create a subscription to the update event feed.
	// If event stream repeatability from a certain point is a requirement,
	// it needs to be further specified with a resource contract.
	SubscribeToUpdate(context.Context, T, Subscriber) (Subscription, error)
}

type DeleterPublisher interface {
	SubscribeToDeleteByID(context.Context, T, Subscriber) (Subscription, error)
	SubscribeToDeleteAll(context.Context, T, Subscriber) (Subscription, error)
}

type Subscription interface {
	io.Closer
}

//type ReplayableSubscription interface {
//	Subscriber
//	ReplayEventsFrom(ctx context.Context, eventID string) error
//}

type Subscriber interface {
	// Handle handles the the subscribed event.
	// Context may or may not have meta information about the received event.
	// To ensure expectations, define a resource specification <contract> about what must be included in the context.
	Handle(ctx context.Context, T interface{}) error
	// Error allow the subscription implementation to communicate unexpected situations that needs to be handled by the subscriber.
	// For e.g. the connection is lost and the subscriber might have cached values
	// that must be invalidated on the next successful Handle call
	Error(ctx context.Context, err error) error
}
