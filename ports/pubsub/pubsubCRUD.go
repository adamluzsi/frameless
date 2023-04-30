package pubsub

import (
	"context"
	"io"

	"github.com/adamluzsi/frameless/pkg/errorkit"
)

type Subscription interface {
	io.Closer
}

type (
	CreateEvent[Entity any] struct{ Entity Entity }

	CreatorSubscriber[Entity any] interface {
		HandleCreateEvent(ctx context.Context, event CreateEvent[Entity]) error
		errorkit.ErrorHandler
	}

	CreatorPublisher[Entity any] interface {
		SubscribeToCreatorEvents(context.Context, CreatorSubscriber[Entity]) (Subscription, error)
	}
)

type (
	UpdateEvent[Entity any] struct{ Entity Entity }

	UpdaterSubscriber[Entity any] interface {
		HandleUpdateEvent(ctx context.Context, event UpdateEvent[Entity]) error
		errorkit.ErrorHandler
	}

	UpdaterPublisher[Entity any] interface {
		SubscribeToUpdaterEvents(context.Context, UpdaterSubscriber[Entity]) (Subscription, error)
	}
)

type (
	DeleteByIDEvent[ID any] struct{ ID ID }
	DeleteAllEvent          struct{}

	DeleterSubscriber[ID any] interface {
		HandleDeleteByIDEvent(ctx context.Context, event DeleteByIDEvent[ID]) error
		HandleDeleteAllEvent(ctx context.Context, event DeleteAllEvent) error
		errorkit.ErrorHandler
	}

	DeleterPublisher[ID any] interface {
		SubscribeToDeleterEvents(context.Context, DeleterSubscriber[ID]) (Subscription, error)
	}
)
