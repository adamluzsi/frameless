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
		var zero Value
		return zero, false
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

func (h ValueHandler[Key, Value]) ContextWithout(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return withoutValueHandler[Key]{Context: ctx}
}

// withoutValueHandler masks a single context key, while every other key keeps
// resolving against the wrapped parent context.
type withoutValueHandler[Key ~struct{}] struct{ context.Context }

func (wo withoutValueHandler[Key]) Value(key any) any {
	if _, ok := key.(Key); ok {
		return nil
	}
	return wo.Context.Value(key)
}

func Detach(parent context.Context) context.Context {
	return WithoutCancel(parent)
}

func WithoutCancel(parent context.Context) context.Context {
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
	mc := &merged{ctxs: ctxs}
	done, cancel := mc.mergeDoneChannels()
	mc.done = done
	return mc, cancel
}

type merged struct {
	ctxs []context.Context

	mu   sync.Mutex
	done <-chan struct{}
	err  error
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	var errs []error
	for _, ctx := range c.ctxs {
		errs = append(errs, ctx.Err())
	}
	return errorkit.Merge(errs...)
}

// setCancel records the cancellation reason for this merged context.
// Called from mergeDoneChannels when the merged context is closed, either
// because one of the source contexts was cancelled or because the cancel
// returned by Merge was invoked.
func (c *merged) setCancel(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.err = err
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

func (mc *merged) mergeDoneChannels() (<-chan struct{}, func()) {
	if len(mc.ctxs) == 0 {
		return nil, func() {}
	}

	var SelectCases []reflect.SelectCase
	for _, ctx := range mc.ctxs {
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
		out   = make(chan struct{})
		onOut sync.Once
	)
	var closeOut = func(err error) {
		onOut.Do(func() {
			mc.setCancel(err)
			close(out)
		})
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			chosen, _, ok := reflect.Select(SelectCases)
			if !ok {
				// A Done channel was closed. If it's the internal `done`
				// channel (chosen == len(ctxs)), the cancel func ran and
				// the source contexts have no error of their own.
				// Otherwise, a source context's Done fired.
				if chosen == len(mc.ctxs) {
					closeOut(context.Canceled)
				} else {
					closeOut(mc.cancellationReason(mc.ctxs))
				}
				return
			}
		}
	})

	var onClose sync.Once
	return out, func() {
		onClose.Do(func() {
			close(done) // signal to break reflect.Select looping
			closeOut(context.Canceled)
		})
		wg.Wait()
	}
}

// cancellationReason returns the first non-nil Err() from the provided contexts.
// Used to attribute a merged-context cancellation to the underlying source
// when one of the sources is cancelled. If none of the sources report an
// error yet (possible due to scheduling), context.Canceled is returned so
// that callers honour the context.Context contract requiring Err() to be
// non-nil after Done() is closed.
func (c *merged) cancellationReason(ctxs []context.Context) error {
	for _, ctx := range ctxs {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return context.Canceled
}

func WithoutValues(ctx context.Context) context.Context {
	return ctxWithoutValues{Context: ctx}
}

type ctxWithoutValues struct{ context.Context }

func (ctxWithoutValues) Value(any) any {
	return nil
}
