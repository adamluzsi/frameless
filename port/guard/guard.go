package guard

import (
	"context"

	"go.llib.dev/frameless/internal/constant"
)

// Locker represents a resource that can be locked and unlocked.
// Implementations must provide a way to acquire the lock, potentially blocking it until it becomes available.
type Locker interface {
	// Lock locks the Locker resource.
	// If the lock is already in use, the calling will be blocked until the locks is available.
	// It returns a context that represent a locked context, and an error that is nil if locking succeeded.
	// The returned context may hold locking related runtime states,
	// It might signal cancellation if the ownership of the lock is lost for some reason.
	Lock(ctx context.Context) (_lockContext context.Context, _ error)
	Unlocker
}

// NonBlockingLocker represents a resource that can be attempted to lock without blocking.
// If the lock is not immediately available, the attempt will fail instead of blocking.
type NonBlockingLocker interface {
	// TryLock attempts to acquire the lock without blocking.
	// Returns true if the lock was acquired successfully, false otherwise.
	//
	// This method does not wait for the lock to become available and instead returns immediately.
	TryLock(ctx context.Context) (_lockContext context.Context, isAcquired bool, _ error)
	Unlocker
}

// Unlocker provides a way to release a previously acquired lock.
type Unlocker interface {
	// Unlock unlocks the Locker resource.
	// It is an error if Locker is not locked on entry to Unlock.
	//
	// It takes the context that Lock returned.
	Unlock(lockContext context.Context) error
}

const ErrNoLock constant.Error = "ErrNoLock"

type LockerFactory[Key comparable] interface {
	// LockerFor returns a Locker associated with the given key.
	//
	// The returned Locker can be used to acquire and release locks on the key.
	// This allows for concurrent access to shared resources, ensuring thread safety.
	//
	// Note: The name "LockerFor" is chosen instead of "LockFor" to emphasize that
	// this method returns a Locker object, which provides locking functionality,
	// rather than directly acquiring a lock.
	LockerFor(Key) Locker
}

type NonBlockingLockerFactory[Key comparable] interface {
	NonBlockingLockerFor(Key) NonBlockingLocker
}
