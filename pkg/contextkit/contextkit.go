package contextkit

import (
	"context"
	"time"
)

type ValueInContext[Key ~struct{}, Value any] struct{}

func (h ValueInContext[Key, Value]) Lookup(ctx context.Context) (Value, bool) {
	if ctx == nil {
		return *new(Value), false
	}
	v, ok := ctx.Value(Key{}).(Value)
	return v, ok
}

func (h ValueInContext[Key, Value]) ContextWith(ctx context.Context, v Value) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, Key{}, v)
}

func Detach(parent context.Context) context.Context {
	return detached{
		Parent: parent,
		Cancel: context.Background(),
	}
}

type detached struct {
	Parent context.Context
	Cancel context.Context
}

func (ctx detached) Deadline() (deadline time.Time, ok bool) {
	return ctx.Cancel.Deadline()
}

func (ctx detached) Done() <-chan struct{} {
	return ctx.Cancel.Done()
}

func (ctx detached) Err() error {
	return ctx.Cancel.Err()
}

func (ctx detached) Value(key any) any {
	return ctx.Parent.Value(key)
}
