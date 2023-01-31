package locks

import (
	"context"

	"github.com/adamluzsi/frameless/internal/consttypes"
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

const ErrNoLock consttypes.Error = `ErrNoLock`

//func FinishLock(l Locker, err *error, lockCtx context.Context) {
//	if err == nil {
//		var ph error
//		err = &ph
//	}
//
//	*err = errorutil.Merge(*err, l.Unlock(lockCtx))
//}
