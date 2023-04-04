package doubles

import (
	"context"

	"github.com/adamluzsi/frameless/ports/pubsub"
)

type StubSubscriber[Entity any, ID any] struct {
	HandleFunc func(ctx context.Context, event interface{}) error
	ErrorFunc  func(ctx context.Context, err error) error
}

func (s StubSubscriber[Entity, ID]) Handle(ctx context.Context, event interface{}) error {
	if s.HandleFunc == nil {
		return nil
	}
	return s.HandleFunc(ctx, event)
}

func (s StubSubscriber[Entity, ID]) HandleCreateEvent(ctx context.Context, event pubsub.CreateEvent[Entity]) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber[Entity, ID]) HandleUpdateEvent(ctx context.Context, event pubsub.UpdateEvent[Entity]) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber[Entity, ID]) HandleDeleteByIDEvent(ctx context.Context, event pubsub.DeleteByIDEvent[ID]) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber[Entity, ID]) HandleDeleteAllEvent(ctx context.Context, event pubsub.DeleteAllEvent) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber[Entity, ID]) HandleError(ctx context.Context, err error) error {
	if s.ErrorFunc == nil {
		return nil
	}
	return s.ErrorFunc(ctx, err)
}
