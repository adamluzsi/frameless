package frameless

import (
	"context"
	"io"
)

type Subscription interface {
	io.Closer
}

type SubscriberErrorHandler interface {
	// Error allow the subscription implementation to be notified about unexpected situations
	// that needs to be handled by the subscriber.
	// For e.g. the connection is lost and the subscriber might have cached values
	// that must be invalidated on the next successful Handle call
	Error(ctx context.Context, err error) error
}

//--------------------------------------------------------------------------------------------------------------------//

type (
	CreateEvent /* [Entity] */ struct{ Entity T }

	CreatorSubscriber /* [Event] */ interface {
		HandleCreateEvent(ctx context.Context, event CreateEvent) error
		SubscriberErrorHandler
	}

	CreatorPublisher /* [EventCreate[Entity]] */ interface {
		SubscribeToCreatorEvents(context.Context, CreatorSubscriber /* [EventCreate[Entity]] */) (Subscription, error)
	}
)

type (
	UpdateEvent /* [Entity] */ struct{ Entity T }

	UpdaterSubscriber /* [Event] */ interface {
		HandleUpdateEvent(ctx context.Context, event UpdateEvent) error
		SubscriberErrorHandler
	}

	UpdaterPublisher interface {
		SubscribeToUpdaterEvents(context.Context, UpdaterSubscriber /* [EventUpdate[Entity]] */) (Subscription, error)
	}
)

type (
	DeleteByIDEvent /* [Entity, ID] */ struct{ ID ID }
	DeleteAllEvent /* [Entity] */      struct{}

	DeleterSubscriber /* [Event] */ interface {
		HandleDeleteByIDEvent(ctx context.Context, event DeleteByIDEvent /* [EventDeleteByID[Entity,ID]] */) error
		HandleDeleteAllEvent(ctx context.Context, event DeleteAllEvent /* [EventDeleteAll[Entity]] */) error
		SubscriberErrorHandler
	}

	DeleterPublisher interface {
		SubscribeToDeleterEvents(context.Context, DeleterSubscriber) (Subscription, error)
	}
)
