package errorkit

import (
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
