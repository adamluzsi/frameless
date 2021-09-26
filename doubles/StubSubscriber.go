package doubles

import (
	"context"

	"github.com/adamluzsi/frameless"
)

type StubSubscriber /*[T]*/ struct {
	HandleFunc func(ctx context.Context, event interface{}) error
	ErrorFunc  func(ctx context.Context, err error) error
}

func (s StubSubscriber) Handle(ctx context.Context, event interface{}) error {
	if s.HandleFunc == nil {
		return nil
	}
	return s.HandleFunc(ctx, event)
}

func (s StubSubscriber) HandleCreateEvent(ctx context.Context, event frameless.CreateEvent) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber) HandleUpdateEvent(ctx context.Context, event frameless.UpdateEvent) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber) HandleDeleteByIDEvent(ctx context.Context, event frameless.DeleteByIDEvent) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber) HandleDeleteAllEvent(ctx context.Context, event frameless.DeleteAllEvent) error {
	return s.Handle(ctx, event)
}

func (s StubSubscriber) HandleError(ctx context.Context, err error) error {
	if s.ErrorFunc == nil {
		return nil
	}
	return s.ErrorFunc(ctx, err)
}
