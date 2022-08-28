package doubles

import (
	"context"
	"github.com/adamluzsi/frameless/ports/pubsub"
)

type StubSubscriber[Ent any, ID any] struct {
	HandleFunc func(ctx context.Context, event interface{}) error
	ErrorFunc  func(ctx context.Context, err error) error
}

func (s StubSubscriber[Ent, ID]) Handle(ctx context.Context, event interface{}) error {
	if s.HandleFunc == nil {
		return nil
	}
	return s.HandleFunc(ctx, event)
}

func (s StubSubscriber[Ent, ID]) HandleCreateEvent(ctx context.Context, event pubsub.CreateEvent[Ent]) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber[Ent, ID]) HandleUpdateEvent(ctx context.Context, event pubsub.UpdateEvent[Ent]) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber[Ent, ID]) HandleDeleteByIDEvent(ctx context.Context, event pubsub.DeleteByIDEvent[ID]) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber[Ent, ID]) HandleDeleteAllEvent(ctx context.Context, event pubsub.DeleteAllEvent) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber[Ent, ID]) HandleError(ctx context.Context, err error) error {
	if s.ErrorFunc == nil {
		return nil
	}
	return s.ErrorFunc(ctx, err)
}
