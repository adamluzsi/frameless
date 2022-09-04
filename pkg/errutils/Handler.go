package errutils

import "context"

// ErrorHandler describes that an object able to handle error use-cases for its purpose.
//
// For e.g. if the component is a pubsub subscription event handler,
// then implementing ErrorHandler means it suppose to handle unexpected use-cases such as connection interruption.
type ErrorHandler interface {
	// HandleError allow the interactor implementation to be notified about unexpected situations.
	HandleError(ctx context.Context, err error) error
}
