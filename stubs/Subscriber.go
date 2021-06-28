package stubs

import "context"

type Subscriber /*[T]*/ struct {
	HandleFunc func(ctx context.Context, event interface{}) error
	ErrorFunc  func(ctx context.Context, err error) error
}

func (s Subscriber) Handle(ctx context.Context, event interface{}) error {
	if s.HandleFunc == nil {
		return nil
	}
	return s.HandleFunc(ctx, event)
}

func (s Subscriber) Error(ctx context.Context, err error) error {
	if s.ErrorFunc == nil {
		return nil
	}
	return s.ErrorFunc(ctx, err)
}
