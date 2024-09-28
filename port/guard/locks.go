package guard

import (
	"context"

	"go.llib.dev/frameless/internal/constant"
)

type Locker interface {
	// Lock locks the Locker resource.
	// If the lock is already in use, the calling will be blocked until the locks is available.
	// It returns a context that represent a locked context, and an error that is nil if locking succeeded.
	// The returned context may hold locking related runtime states,
	// It might signal cancellation if the ownership of the lock is lost for some reason.
	Lock(ctx context.Context) (_lockCtx context.Context, _ error)
	// Unlock unlocks the Locker resource.
	// It is an error if Locker is not locked on entry to Unlock.
	//
	// It takes the context that Lock returned.
	Unlock(lockCtx context.Context) error
}

const ErrNoLock constant.Error = "ErrNoLock"

type LockerFactory[Key comparable] interface {
	LockerFor(Key) Locker
}

type FactoryFunc[Key comparable] func(Key) Locker

func (fn FactoryFunc[Key]) LockerFor(key Key) Locker { return fn(key) }
