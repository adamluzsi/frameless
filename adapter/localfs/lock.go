package localfs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/testcase/clock"
)

// Locker serialises access to an existing directory.
// On Linux and macOS it uses flock(2) on the directory; on Windows it uses
// LockFileEx on the directory's persistent, hidden ".lock" file.
// On other platforms it only coordinates lockers within the same process.
// Separate Locker instances for the same directory contend for the same lock.
//
// Native locks are advisory: every participant must use the same locking protocol.
// The directory (and, on Windows, its ".lock" file) must not be removed or replaced
// while in use. Locks have no lease and are held through open handles until Unlock
// or process exit; filesystem-specific locking guarantees still apply.
//
// Use Locker through a pointer and do not copy or change its configuration after use.
// The zero FS is unjailed; Dir must be set and must already exist.
//
//	locker := &localfs.Locker{FS: localfs.FileSystem{RootPath: root}, Dir: "settings"}
//
// Cancellation does not release a held lock: callers must finish their protected
// cleanup before calling Unlock, even if the lock context has been cancelled.
// Locker does not monitor external removal or replacement of its directory.
type Locker struct {
	// FS resolves Dir, respecting FileSystem.RootPath when it is set.
	FS FileSystem
	// Dir is the existing directory to lock, resolved within FS.
	Dir string

	lock dirLock
}

var _ guard.Locker = (*Locker)(nil)

// Lock waits for exclusive access to Dir, or until ctx is cancelled.
// The returned context carries ownership for this Locker instance. Passing it
// (or a derived context) to Lock again is a re-entry, not another acquisition.
func (l *Locker) Lock(ctx context.Context) (context.Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("localfs: missing lock context")
	}
	if err := lockContextError(ctx); err != nil {
		return nil, err
	}
	if state, ok := l.lookupLock(ctx); ok {
		state.mutex.Lock()
		defer state.mutex.Unlock()
		if err := state.check(); err != nil {
			return nil, err
		}
		return ctx, nil
	}
	root, err := l.openLockDir()
	if err != nil {
		return nil, err
	}
	unlock, err := l.lock.lock(ctx, root)
	if err != nil {
		return nil, errors.Join(err, root.Close())
	}
	// Cancellation may race with either the semaphore or the native lock acquisition.
	if err := lockContextError(ctx); err != nil {
		return nil, errors.Join(err, unlock(), root.Close())
	}
	ctx, cancel := context.WithCancelCause(ctx)
	state := &lockerState{
		ctx:    ctx,
		cancel: cancel,
		root:   root,
		unlock: func() error { return errors.Join(unlock(), root.Close()) },
	}
	ctx = context.WithValue(ctx, lockerContextKey{l}, state)
	return context.WithValue(ctx, directoryLockKey{l.FS, l.Dir}, state), nil
}

// Unlock releases ownership synchronously, even if ctx is cancelled, and cancels
// the lock context. A cancelled context's error is reported after releasing.
// Repeated or concurrent calls with the same ownership return the first result
// without releasing a later acquisition. A foreign context returns guard.ErrNoLock.
func (l *Locker) Unlock(ctx context.Context) error {
	state, ok := l.lookupLock(ctx)
	if !ok {
		return guard.ErrNoLock
	}
	state.mutex.Lock()
	defer state.mutex.Unlock()
	if !state.done {
		err := lockContextError(ctx)
		if cause := context.Cause(state.ctx); errors.Is(cause, guard.ErrLockLost) {
			err = cause
		}
		state.err = errors.Join(err, state.unlock())
		state.done = true
		state.cancel(nil)
	}
	return state.err
}

func (l *Locker) openLockDir() (*os.Root, error) { return l.FS.openRoot(l.Dir) }

type lockerContextKey struct{ locker *Locker }

// Unlike the instance key, this key lets Repository find the pinned directory
// through a Locker decorator which preserves the returned context's values.
// Match configuration without resolving paths again: aliases may have moved.
type directoryLockKey struct {
	fs  FileSystem
	dir string
}

type lockerState struct {
	mutex  sync.Mutex
	done   bool
	err    error
	ctx    context.Context
	cancel context.CancelCauseFunc
	root   *os.Root
	unlock func() error
}

// check is called with mutex held, including when cancellation was detached
// from a borrowed context: that must not revive ended or lost ownership.
func (s *lockerState) check() error {
	if s.done {
		return guard.ErrNoLock
	}
	return lockContextError(s.ctx)
}

func (s *lockerState) lockError() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.check()
}

func (l *Locker) lookupLock(ctx context.Context) (*lockerState, bool) {
	if ctx == nil {
		return nil, false
	}
	state, ok := ctx.Value(lockerContextKey{l}).(*lockerState)
	return state, ok
}

func lockContextError(ctx context.Context) error {
	if cause := context.Cause(ctx); errors.Is(cause, guard.ErrLockLost) {
		return cause
	}
	return ctx.Err()
}

// acquireRepositoryLock forwards cancellation only while waiting. After Lock
// returns, its ownership context lives until release, even if a child operation
// is cancelled. Stopping/joining the callback closes the acquisition handoff race.
func acquireRepositoryLock(ctx context.Context, locker guard.Locker) (context.Context, func() error, error) {
	if err := lockContextError(ctx); err != nil {
		return nil, nil, err
	}
	waiting, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		cancel(context.Cause(ctx))
		close(cancelled)
	})
	lockCtx, err := locker.Lock(waiting)
	if !stop() {
		<-cancelled
	}
	if err != nil {
		cancel(nil)
		return nil, nil, errors.Join(lockContextError(ctx), err)
	}
	var release func() error
	if lockCtx == waiting {
		// The guard re-entry idiom returns its input unchanged. This ownership
		// belongs to the caller, whose original context remains authoritative.
		cancel(nil)
		lockCtx = ctx
		release = func() error { return lockContextError(lockCtx) }
	} else {
		release = func() error {
			defer cancel(nil)
			return errors.Join(lockContextError(lockCtx), locker.Unlock(context.WithoutCancel(lockCtx)))
		}
	}
	if err := errors.Join(lockContextError(ctx), lockContextError(lockCtx)); err != nil {
		return nil, nil, errors.Join(err, release())
	}
	return lockCtx, release, nil
}

// dirLock serialises the changes of a directory.
// Between the goroutines of the process, it is a mutex,
// and on the platforms which support it, it locks the directory between processes as well (see lockDir).
//
// A dirLock must not be copied after its first use.
type dirLock struct {
	init sync.Once
	// turn has room for a single holder of the lock.
	// Unlike a sync.Mutex, waiting for it can stop when a context is done.
	turn chan struct{}
}

// take waits for this instance's turn, without acquiring a native directory lock.
func (l *dirLock) take(ctx context.Context) (func(), error) {
	l.init.Do(func() { l.turn = make(chan struct{}, 1) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case l.turn <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		<-l.turn
		return nil, err
	}
	return func() { <-l.turn }, nil
}

// lock also acquires the native lock of root, using the same pinned directory.
func (l *dirLock) lock(ctx context.Context, root *os.Root) (unlock func() error, _ error) {
	release, err := l.take(ctx)
	if err != nil {
		return nil, err
	}
	unlockDir, err := lockDir(ctx, root)
	if err != nil {
		release()
		return nil, err
	}
	return func() error {
		defer release()
		return unlockDir()
	}, nil
}

// pollLock calls tryLock until it takes the lock, or until it fails, or until ctx is done.
// When tryLock reports that the lock is busy, as another process holds it,
// it is called again after a wait, which grows from a millisecond up to maxLockPoll.
func pollLock(ctx context.Context, tryLock func() (busy bool, _ error)) error {
	for wait := time.Millisecond; ; wait = min(2*wait, maxLockPoll) {
		if busy, err := tryLock(); err != nil || !busy {
			return err
		}
		select {
		case <-clock.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// maxLockPoll is the longest wait between two attempts to lock a directory, which is locked by another process.
const maxLockPoll = 25 * time.Millisecond
