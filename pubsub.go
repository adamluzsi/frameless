package frameless

import (
	"context"
	"io"
)

type Subscription interface {
	io.Closer
}

type (
	CreateEvent/* [Entity] */ struct{ Entity T }

	CreatorSubscriber/* [Event] */ interface {
		HandleCreateEvent(ctx context.Context, event CreateEvent) error
		ErrorHandler
	}

	CreatorPublisher/* [EventCreate[Entity]] */ interface {
		SubscribeToCreatorEvents(context.Context, CreatorSubscriber /* [EventCreate[Entity]] */) (Subscription, error)
	}
)

type (
	UpdateEvent/* [Entity] */ struct{ Entity T }

	UpdaterSubscriber/* [Event] */ interface {
		HandleUpdateEvent(ctx context.Context, event UpdateEvent) error
		ErrorHandler
	}

	UpdaterPublisher interface {
		SubscribeToUpdaterEvents(context.Context, UpdaterSubscriber /* [EventUpdate[Entity]] */) (Subscription, error)
	}
)

type (
	DeleteByIDEvent/* [Entity, ID] */ struct{ ID ID }
	DeleteAllEvent/* [Entity] */ struct{}

	DeleterSubscriber/* [Event] */ interface {
		HandleDeleteByIDEvent(ctx context.Context, event DeleteByIDEvent /* [EventDeleteByID[Entity,ID]] */) error
		HandleDeleteAllEvent(ctx context.Context, event DeleteAllEvent /* [EventDeleteAll[Entity]] */) error
		ErrorHandler
	}

	DeleterPublisher interface {
		SubscribeToDeleterEvents(context.Context, DeleterSubscriber) (Subscription, error)
	}
)
