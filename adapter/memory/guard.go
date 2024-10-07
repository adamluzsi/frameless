package memory

import (
	"context"
	"fmt"
	"sync"

	"go.llib.dev/frameless/port/guard"
)

func NewLocker() *Lock { return &Lock{} }

// Lock is a memory-based implementation of guard.Lock.
// Lock is not safe to call from different application instances.
// Lock is meant to be used in a single application instance.
type Lock struct{ mutex sync.Mutex }

type ctxKeyLock struct{}
type ctxValueLock struct {
	done   bool
	cancel func()
	unlock func()

	onUnlock sync.Once
}

func (l *ctxValueLock) Unlock() {
	l.onUnlock.Do(func() {
		l.done = true
		l.cancel()
	})
}

func (l *Lock) Lock(ctx context.Context) (context.Context, error) {
	if ok, err := l.isLockedAlready(ctx); err != nil {
		return nil, err
	} else if ok {
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
	return l.makeLockContext(ctx), nil
}

func (l *Lock) TryLock(ctx context.Context) (context.Context, bool, error) {
	if ok, err := l.isLockedAlready(ctx); err != nil {
		return nil, false, err
	} else if ok {
		return ctx, true, nil
	}
	if !l.mutex.TryLock() {
		return nil, false, nil
	}
	return l.makeLockContext(ctx), true, nil
}

func (l *Lock) isLockedAlready(ctx context.Context) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("missing context")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if _, ok := l.lookup(ctx); ok {
		return true, nil
	}
	return false, nil
}

func (l *Lock) makeLockContext(ctx context.Context) context.Context {
	var onUnlock sync.Once
	var unlock = func() { onUnlock.Do(func() { l.mutex.Unlock() }) }
	ctx, cancel := context.WithCancel(ctx)
	context.AfterFunc(ctx, unlock)
	return context.WithValue(ctx, ctxKeyLock{}, &ctxValueLock{cancel: cancel})
}

func (l *Lock) Unlock(ctx context.Context) error {
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

func (l *Lock) lookup(ctx context.Context) (*ctxValueLock, bool) {
	lockState, ok := ctx.Value(ctxKeyLock{}).(*ctxValueLock)
	return lockState, ok
}

func NewLockerFactory[Key comparable]() *LockerFactory[Key] {
	return &LockerFactory[Key]{}
}

type LockerFactory[Key comparable] struct {
	locks map[Key]*Lock
	mutex sync.Mutex
}

func (lf *LockerFactory[Key]) LockerFor(key Key) guard.Locker {
	return lf.lockFor(key)
}

func (lf *LockerFactory[Key]) NonBlockingLockerFor(key Key) guard.NonBlockingLocker {
	return lf.lockFor(key)
}

func (lf *LockerFactory[Key]) lockFor(key Key) *Lock {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()
	if lf.locks == nil {
		lf.locks = make(map[Key]*Lock)
	}
	if _, ok := lf.locks[key]; !ok {
		lf.locks[key] = NewLocker()
	}
	return lf.locks[key]
}
