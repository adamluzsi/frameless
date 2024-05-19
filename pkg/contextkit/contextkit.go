package contextkit

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
)

type ValueHandler[Key ~struct{}, Value any] struct{}

func (h ValueHandler[Key, Value]) Lookup(ctx context.Context) (Value, bool) {
	if ctx == nil {
		return *new(Value), false
	}
	v, ok := ctx.Value(Key{}).(Value)
	return v, ok
}

func (h ValueHandler[Key, Value]) ContextWith(ctx context.Context, v Value) context.Context {
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

func Merge(ctxs ...context.Context) (context.Context, func()) {
	switch len(ctxs) {
	case 0:
		return context.Background(), func() {}
	case 1:
		return ctxs[0], func() {}
	}
	var (
		ctx    context.Context
		cancel func() = func() {}
		done          = make(chan struct{})
	)
	for i := len(ctxs) - 1; 0 <= i; i-- {
		if ctx == nil {
			ctx = ctxs[i]
			continue
		}
		m := &merged{
			p:    ctx,
			m:    ctxs[i],
			done: done,
		}
		m.Init()
		og := cancel
		cancel = func() {
			og()
			m.Cancel()
		}
		ctx = m
	}
	return ctx, cancel
}

type merged struct {
	p, m context.Context
	done chan struct{}
}

func (c *merged) Init() {
	go func() {
		select {
		case <-c.done:
		case <-c.p.Done():
			c.Cancel()
		case <-c.m.Done():
			c.Cancel()
		}
	}()
}

func (c *merged) Cancel() {
	if c.done == nil {
		return
	}
	defer func() { _ = recover() }()
	close(c.done)
}

func (c *merged) Deadline() (deadline time.Time, ok bool) {
	if dl, ok := c.p.Deadline(); ok {
		return dl, ok
	}
	return c.m.Deadline()
}

func (c *merged) Done() <-chan struct{} {
	return c.done
}

func (c *merged) Err() error {
	return errorkit.Merge(c.p.Err(), c.m.Err())
}

func (c *merged) Value(key any) any {
	if v := c.p.Value(key); v != nil {
		return v
	}
	return c.m.Value(key)
}
