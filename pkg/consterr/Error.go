package consterr

import "context"

// Error is an implementation for the error interface that allow you to declare exported globals with the `const` keyword.
//
//   TL;DR:
//     const ErrSomething errs.Error = "something is an error"
type Error string

// Error implement the error interface
func (err Error) Error() string { return string(err) }

// ErrorHandler describes that an object able to handle error use-cases for its purpose.
//
// For e.g. if the component is a pubsub subscription event handler,
// then implementing ErrorHandler means it suppose to handle unexpected use-cases such as connection interruption.
type ErrorHandler interface {
	// HandleError allow the interactor implementation to be notified about unexpected situations.
	HandleError(ctx context.Context, err error) error
}
