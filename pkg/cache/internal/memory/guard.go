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
type Lock struct {
	init sync.Once
	// sem is a capacity-one semaphore channel rather than a sync.Mutex,
	// because it lets Lock park the goroutine in a select that also watches
	// the context. A sync.Mutex can only be waited on by blocking
	// uninterruptibly or by spinning on TryLock, and spinning keeps a CPU
	// core busy for as long as the lock is held.
	sem chan struct{}
	key any
}

func (l *Lock) semaphore() chan struct{} {
	l.init.Do(func() { l.sem = make(chan struct{}, 1) })
	return l.sem
}

type ctxKeyLock struct{ Key any }

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
		l.unlock()
	})
}

func (l *Lock) Lock(ctx context.Context) (context.Context, error) {
	if ok, err := l.isLockedAlready(ctx); err != nil {
		return nil, err
	} else if ok {
		return ctx, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case l.semaphore() <- struct{}{}:
		return l.makeLockContext(ctx), nil
	}
}

func (l *Lock) TryLock(ctx context.Context) (context.Context, bool, error) {
	if ok, err := l.isLockedAlready(ctx); err != nil {
		return nil, false, err
	} else if ok {
		return ctx, true, nil
	}
	select {
	case l.semaphore() <- struct{}{}:
		return l.makeLockContext(ctx), true, nil
	default:
		return nil, false, nil
	}
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
	ctx, cancel := context.WithCancel(ctx)
	sem := l.semaphore()
	// Keep the lock release inline with the cancel so that callers see
	// the lock as released the moment Unlock returns.
	// context.AfterFunc would defer the release to a separate
	// goroutine, which races with the next TryLock attempt and causes
	// spurious ErrAlreadyRunningProcess failures.
	return context.WithValue(ctx, ctxKeyLock{Key: l.key}, &ctxValueLock{
		cancel: cancel,
		unlock: func() { <-sem },
	})
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
	// Surface the context error to the caller (e.g. when the parent context
	// was cancelled mid-lock), but still release the mutex so the next
	// caller can acquire the lock.
	if err := ctx.Err(); err != nil {
		lockState.Unlock()
		return err
	}
	lockState.Unlock()
	return nil
}

func (l *Lock) lookup(ctx context.Context) (*ctxValueLock, bool) {
	lockState, ok := ctx.Value(ctxKeyLock{Key: l.key}).(*ctxValueLock)
	return lockState, ok
}

func NewLockerFactory[Key comparable, Locker guard.Unlocker]() *LockerFactory[Key, Locker] {
	return &LockerFactory[Key, Locker]{}
}

type LockerFactory[Key comparable, Locker guard.Unlocker] struct {
	locks map[Key]*Lock
	mutex sync.Mutex
}

func (lf *LockerFactory[Key, Locker]) LockerFor(key Key) Locker {
	return any(lf.lockFor(key)).(Locker)
}

func (lf *LockerFactory[Key, Locker]) NonBlockingLockerFor(key Key) guard.NonBlockingLocker {
	return lf.lockFor(key)
}

func (lf *LockerFactory[Key, Locker]) lockFor(key Key) *Lock {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()
	if lf.locks == nil {
		lf.locks = make(map[Key]*Lock)
	}
	if _, ok := lf.locks[key]; !ok {
		locker := NewLocker()
		locker.key = key
		lf.locks[key] = locker
	}
	return lf.locks[key]
}
