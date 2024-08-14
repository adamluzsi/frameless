package memory

import (
	"context"
	"fmt"
	"sync"

	"go.llib.dev/frameless/port/guard"
)

func NewLocker() *Locker { return &Locker{} }

// Locker is a memory-based implementation of guard.Locker.
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
		unlock func()

		onUnlock sync.Once
	}
)

func (l *ctxValueLock) Unlock() {
	l.onUnlock.Do(func() {
		l.done = true
		l.cancel()
	})
}

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

	var onUnlock sync.Once
	var unlock = func() {
		onUnlock.Do(func() { l.mutex.Unlock() })
	}

	ctx, cancel := context.WithCancel(ctx)
	context.AfterFunc(ctx, unlock)
	return context.WithValue(ctx, ctxKeyLock{}, &ctxValueLock{cancel: cancel}), nil
}

func (l *Locker) Unlock(ctx context.Context) error {
	if ctx == nil {
		return guard.ErrNoLock
	}
	lockState, ok := l.lookup(ctx)
	if !ok {
		return guard.ErrNoLock
	}
	if lockState.done {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lockState.Unlock()
	return nil
}

func (l *Locker) lookup(ctx context.Context) (*ctxValueLock, bool) {
	lockState, ok := ctx.Value(ctxKeyLock{}).(*ctxValueLock)
	return lockState, ok
}

func NewLockerFactory[Key comparable]() *LockerFactory[Key] {
	return &LockerFactory[Key]{}
}

type LockerFactory[Key comparable] struct {
	locks map[Key]*Locker
	mutex sync.Mutex
}

func (lf *LockerFactory[Key]) LockerFor(key Key) guard.Locker {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()
	if lf.locks == nil {
		lf.locks = make(map[Key]*Locker)
	}
	if _, ok := lf.locks[key]; !ok {
		lf.locks[key] = NewLocker()
	}
	return lf.locks[key]
}
