package frameless

import (
	"context"
	"io"
)

//------------------------------------------------------ C/R/U/D -----------------------------------------------------//

type Creator /* T */ interface {
	// Create takes a ptr to a entity<T> and store it into the resource.
	// It also updates the entity<T> ext:"ID" field with the associated uniq resource id.
	// The reason behind this links the id and not returning the id is that,
	// in most case the Create error value is the only thing that is checked for errors,
	// and introducing an extra value also introduce boiler plates in the handling.
	Create(ctx context.Context, ptr /* *T */ interface{}) error
}

type Finder /* T, ID */ interface {
	// FindByID will link an entity that is found in the resource to the received ptr,
	// and report back if it succeeded finding the entity in the resource.
	// It also reports if there was an unexpected exception during the execution.
	// It was an intentional decision to not use error to represent "not found" case,
	// but tell explicitly this information in the form of return bool value.
	FindByID(ctx context.Context, ptr /* *T */, id /* ID */ interface{}) (found bool, err error)
	// FindAll will return all entity that has <T> type
	FindAll(context.Context) Iterator
}

type Updater /* T */ interface {
	// Update will takes a ptr that points to an entity
	// and update the corresponding stored entity with the received entity field values
	Update(ctx context.Context, ptr /* *T */ interface{}) error
}

// Deleter request to destroy a business entity in the Resource that implement it's test.
type Deleter /* T */ interface {
	// DeleteByID will remove a <T> type entity from the storage by a given ID
	DeleteByID(ctx context.Context, id interface{}) error
	// DeleteAll will erase all entity from the resource that has <T> type
	DeleteAll(context.Context) error
}

//------------------------------------------------------ Pub/Sub -----------------------------------------------------//

type CreatorPublisher /* T */ interface {
	// SubscribeToCreate create a subscription to create event feed.
	// 	eg.: storage.SubscribeToCreate(``, cache.CreateEventHandler())
	//
	// If event stream repeatability from a certain point is a requirement,
	// it needs to be further specified with a resource contract.
	SubscribeToCreate(context.Context, Subscriber) (Subscription, error)
}

type UpdaterPublisher /* T */ interface {
	// SubscribeToUpdate create a subscription to the update event feed.
	// If event stream repeatability from a certain point is a requirement,
	// it needs to be further specified with a resource contract.
	SubscribeToUpdate(context.Context, Subscriber) (Subscription, error)
}

type DeleterPublisher /* T */ interface {
	SubscribeToDeleteByID(context.Context, Subscriber) (Subscription, error)
	SubscribeToDeleteAll(context.Context, Subscriber) (Subscription, error)
}

type Subscription interface {
	io.Closer
}

type Subscriber /* T */ interface {
	// Handle handles the the subscribed event.
	// Context may or may not have meta information about the received event.
	// To ensure expectations, define a resource specification <contract> about what must be included in the context.
	Handle(ctx context.Context, ent /* T */ interface{}) error
	// Error allow the subscription implementation to be notified about unexpected situations
	// that needs to be handled by the subscriber.
	// For e.g. the connection is lost and the subscriber might have cached values
	// that must be invalidated on the next successful Handle call
	Error(ctx context.Context, err error) error
}
