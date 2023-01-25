package memory

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/ports/locks"
	"sync"
)

func NewLocker() *Locker { return &Locker{} }

// Locker is a memory-based shared mutex implementation.
// Locker is not safe to call from different application instances.
// Locker is meant to be used in a single application instance.
type Locker struct {
	mutex sync.Mutex
}

type (
	ctxKeyLock   struct{}
	ctxValueLock struct {
		done   bool
		cancel func()
	}
)

func (l *Locker) Lock(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("missing context")
	}
	if _, ok := l.lookup(ctx); ok {
		return ctx, nil
	}
tryLock:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if l.mutex.TryLock() {
				break tryLock
			}
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	return context.WithValue(ctx, ctxKeyLock{}, &ctxValueLock{cancel: cancel}), nil
}

func (l *Locker) Unlock(ctx context.Context) error {
	if ctx == nil {
		return locks.ErrNoLock
	}
	lockState, ok := l.lookup(ctx)
	if !ok {
		return locks.ErrNoLock
	}
	if lockState.done {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lockState.done = true
	lockState.cancel()
	l.mutex.Unlock()
	return nil
}

func (l *Locker) lookup(ctx context.Context) (*ctxValueLock, bool) {
	lockState, ok := ctx.Value(ctxKeyLock{}).(*ctxValueLock)
	return lockState, ok
}
