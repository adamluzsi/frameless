package errorkit

// Finish is a helper function that can be used from a deferred context.
//
// Usage:
//   defer errorkit.Finish(&returnError, rows.Close)
//
func Finish(returnErr *error, blk func() error) {
	*returnErr = Merge(*returnErr, blk())
}
