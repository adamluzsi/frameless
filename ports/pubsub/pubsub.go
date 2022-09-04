package pubsub

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/errutils"
	"io"
)

type Subscription interface {
	io.Closer
}

type (
	CreateEvent[Ent any] struct{ Entity Ent }

	CreatorSubscriber[Ent any] interface {
		HandleCreateEvent(ctx context.Context, event CreateEvent[Ent]) error
		errutils.ErrorHandler
	}

	CreatorPublisher[Ent any] interface {
		SubscribeToCreatorEvents(context.Context, CreatorSubscriber[Ent]) (Subscription, error)
	}
)

type (
	UpdateEvent[Ent any] struct{ Entity Ent }

	UpdaterSubscriber[Ent any] interface {
		HandleUpdateEvent(ctx context.Context, event UpdateEvent[Ent]) error
		errutils.ErrorHandler
	}

	UpdaterPublisher[Ent any] interface {
		SubscribeToUpdaterEvents(context.Context, UpdaterSubscriber[Ent]) (Subscription, error)
	}
)

type (
	DeleteByIDEvent[ID any] struct{ ID ID }
	DeleteAllEvent          struct{}

	DeleterSubscriber[ID any] interface {
		HandleDeleteByIDEvent(ctx context.Context, event DeleteByIDEvent[ID]) error
		HandleDeleteAllEvent(ctx context.Context, event DeleteAllEvent) error
		errutils.ErrorHandler
	}

	DeleterPublisher[ID any] interface {
		SubscribeToDeleterEvents(context.Context, DeleterSubscriber[ID]) (Subscription, error)
	}
)
