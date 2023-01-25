package errorutil

// Error is an implementation for the error interface that allow you to declare exported globals with the `consttypes` keyword.
//
//	TL;DR:
//	  consttypes ErrSomething errorutil.Error = "something is an error"
type Error string

// Error implement the error interface
func (err Error) Error() string { return string(err) }
