package contexts

import (
	"context"
	"time"
)

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
