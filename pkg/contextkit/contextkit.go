package contextkit

import (
	"context"
	"reflect"
	"sync"
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

// Merge combines multiple contexts into one.
// The merged context will include all values from the source contexts.
// If any source context is cancelled, the merged context will be cancelled.
// If multiple source contexts have deadlines, the nearest deadline will be used for the merged context.
// The second function argument must be deferred to prevent goroutine leaks.
func Merge(ctxs ...context.Context) (context.Context, func()) {
	switch len(ctxs) {
	case 0:
		return context.Background(), func() {}
	case 1:
		return ctxs[0], func() {}
	}
	done, cancel := mergeDoneChannels(ctxs...)
	return &merged{
		ctxs: ctxs,
		done: done,
	}, cancel
}

type merged struct {
	ctxs []context.Context
	done <-chan struct{}
}

func (c *merged) Deadline() (time.Time, bool) {
	var (
		deadline time.Time
		rok      bool
	)
	for _, ctx := range c.ctxs {
		if dl, ok := ctx.Deadline(); ok {
			rok = true
			if deadline.IsZero() {
				deadline = dl

			}
			if dl.Before(deadline) { // return the smalest deadline
				deadline = dl
			}
		}
	}
	return deadline, rok
}

func (c *merged) Done() <-chan struct{} {
	return c.done
}

func (c *merged) Err() error {
	var errs []error
	for _, ctx := range c.ctxs {
		errs = append(errs, ctx.Err())
	}
	return errorkit.Merge(errs...)
}

func (c *merged) Value(key any) any {
	for i := len(c.ctxs) - 1; 0 <= i; i-- {
		val := c.ctxs[i].Value(key)
		if val != nil {
			return val
		}
	}
	return nil
}

func mergeDoneChannels(ctxs ...context.Context) (<-chan struct{}, func()) {
	if len(ctxs) == 0 {
		return nil, func() {}
	}

	var SelectCases []reflect.SelectCase
	for _, ctx := range ctxs {
		SelectCases = append(SelectCases, reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(ctx.Done()),
		})
	}

	done := make(chan struct{})
	// we register done as well, to ensure the inf loop can be broke by reflect.Select
	SelectCases = append(SelectCases, reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(done),
	})

	var (
		out      = make(chan struct{})
		onOut    sync.Once
		closeOut = func() { onOut.Do(func() { close(out) }) }
	)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			if _, _, ok := reflect.Select(SelectCases); !ok {
				// ctx.Done() close signal received
				closeOut()
				return
			}
		}
	}()

	var onClose sync.Once
	return out, func() {
		onClose.Do(func() {
			close(done) // signal to break reflect.Select looping
			closeOut()
		})
		wg.Wait()
	}
}
