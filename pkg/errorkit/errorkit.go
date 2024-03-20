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

// As function serves as a shorthand to enable one-liner error handling with errors.As.
// It's meant to be used within an if statement, much like Lookup functions such as os.LookupEnv.
func As[T error](err error) (T, bool) {
	var v T
	ok := errors.As(err, &v)
	return v, ok
}
