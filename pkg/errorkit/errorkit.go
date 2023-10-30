package errorkit

import "go.llib.dev/frameless/pkg/merge"

// Finish is a helper function that can be used from a deferred context.
//
// Usage:
//
//	defer errorkit.Finish(&returnError, rows.Close)
func Finish(returnErr *error, blk func() error) {
	*returnErr = Merge(*returnErr, blk())
}

// Merge will combine all given non nil error values into a single error value.
// If no valid error is given, nil is returned.
// If only a single non nil error value is given, the error value is returned.
func Merge(errs ...error) error {
	return merge.Error(errs...)
}
