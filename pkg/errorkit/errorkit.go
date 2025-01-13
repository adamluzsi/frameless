package errorkit

import (
	"errors"
	"fmt"
)

// Finish is a helper function that can be used from a deferred context.
//
// Usage:
//
//	defer errorkit.Finish(&returnError, rows.Close)
func Finish(returnErr *error, blk func() error) {
	*returnErr = Merge(*returnErr, blk())
}

// FinishOnError is a helper function that can be used from a deferred context.
// It runs the block conditionally, when the return error, which was assigned by the `return` keyword is not nil.
//
// Usage:
//
//	defer errorkit.FinishOnError(&returnError, func() { rollback(ctx) })
func FinishOnError(returnErr *error, blk func()) {
	if returnErr == nil || *returnErr == nil {
		return
	}
	blk()
}

// Recover will attempt a recover, and if recovery yields a value, it sets it as an error.
func Recover(returnErr *error) {
	r := recover()
	if r == nil {
		return
	}
	switch r := r.(type) {
	case error:
		*returnErr = r
	default:
		*returnErr = fmt.Errorf("%v", r)
	}
}

// RecoverWith will attempt a recover, and if recovery yields a non nil value, it executs the passed function.
func RecoverWith(blk func(r any)) {
	r := recover()
	if r == nil {
		return
	}
	blk(r)
}

// As function serves as a shorthand to enable one-liner error handling with errors.As.
// It's meant to be used within an if statement, much like Lookup functions such as os.LookupEnv.
func As[T error](err error) (T, bool) {
	var v T
	ok := errors.As(err, &v)
	return v, ok
}
