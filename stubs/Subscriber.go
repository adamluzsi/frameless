package stubs

import "context"

type Subscriber /*[T]*/ struct {
	HandleFunc func(ctx context.Context, ent interface{} /* T */) error
	ErrorFunc  func(ctx context.Context, err error) error
}

func (s Subscriber) Handle(ctx context.Context, ent interface{} /* T */) error {
	if s.HandleFunc == nil {
		return nil
	}
	return s.HandleFunc(ctx, ent)
}

func (s Subscriber) Error(ctx context.Context, err error) error {
	if s.ErrorFunc == nil {
		return nil
	}
	return s.ErrorFunc(ctx, err)
}
