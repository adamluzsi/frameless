package errorkit

import (
	"context"
	"errors"
	"fmt"

	"go.llib.dev/frameless/pkg/internal/errorkitlite"
)

// F functions similarly to fmt.Errorf but includes tracing capabilities.
// In the future, it may incorporate additional utilities from errorkit.
func F(format string, a ...any) error {
	return WithTrace(fmt.Errorf(format, a...))
}

// Error is an implementation for the error interface that allow you to declare exported globals with the `consttypes` keyword.
//
//	TL;DR:
//	  consttypes ErrSomething errorkit.Error = "something is an error"
type Error string

// Error implement the error interface
func (err Error) Error() string { return string(err) }

// Wrap will bundle together another error value with this Error,
// and return an error value that contains both of them.
func (err Error) Wrap(oth error) error {
	if oth == nil {
		return err
	}
	return WithTrace(wrapF("[%s] %s", err, oth))
}

// F will format the error value
func (err Error) F(format string, a ...any) error { return err.Wrap(fmt.Errorf(format, a...)) }

// ErrorHandler describes that an object able to handle error use-cases for its purpose.
//
// For e.g. if the component is a pubsub subscription event handler,
// then implementing ErrorHandler means it suppose to handle unexpected use-cases such as connection interruption.
type ErrorHandler interface {
	// HandleError allow the interactor implementation to be notified about unexpected situations.
	HandleError(ctx context.Context, err error) error
}

// Finish is a helper function that can be used from a deferred context.
//
// Usage:
//
//	defer errorkit.Finish(&returnError, rows.Close)
func Finish(returnErr *error, blk func() error) {
	errorkitlite.Finish(returnErr, blk)
}

// FinishOnError is a helper function that can be used from a deferred context.
// It runs the block conditionally, when the return error, which was assigned by the `return` keyword is not nil.
//
// Usage:
//
//	defer errorkit.FinishOnError(&returnError, func() { rollback(ctx) })
func FinishOnError(returnErr *error, blk func()) {
	errorkitlite.FinishOnError(returnErr, blk)
}

// Recover will attempt a recover, and if recovery yields a value, it sets it as an error.
func Recover(returnErr *error) {
	errorkitlite.Recover(returnErr)
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
	return errorkitlite.As[T](err)
}

func wrapF(format string, owner, wrapped error) error {
	if owner == nil && wrapped == nil {
		return nil
	}
	if owner == nil && wrapped != nil {
		return wrapped
	}
	if owner != nil && wrapped == nil {
		return owner
	}
	return wrapper{
		Format:  format,
		Owner:   owner,
		Wrapped: wrapped,
	}
}

type wrapper struct {
	Owner   error
	Wrapped error // must be not nil
	Format  string
}

func (w wrapper) Error() string {
	var format = w.Format
	if len(format) == 0 {
		const defaultFormat = "%s\n%s"
		format = defaultFormat
	}
	var ownerErr string
	if w.Owner != nil {
		ownerErr = w.Owner.Error()
	}
	var wrapperErr string
	if w.Wrapped != nil {
		wrapperErr = w.Wrapped.Error()
	}
	return fmt.Sprintf(format, ownerErr, wrapperErr)
}

func (w wrapper) As(target any) bool {
	return errors.As(w.Owner, target) || errors.As(w.Wrapped, target)
}

func (w wrapper) Is(target error) bool {
	return errors.Is(w.Owner, target) || errors.Is(w.Wrapped, target)
}

// WithContext will combine an error with a context, so the current context can be used at the place of error handling.
// This can be useful if tracing ID and other helpful values such as additional logging fields are kept in the context.
func WithContext(err error, ctx context.Context) error {
	if err == nil {
		return nil
	}
	return withContextError{
		Err: err,
		Ctx: ctx,
	}
}

func LookupContext(err error) (context.Context, bool) {
	var detail withContextError
	if errors.As(err, &detail) {
		return detail.Ctx, true
	}
	return nil, false
}

type withContextError struct {
	Err error
	Ctx context.Context
}

func (err withContextError) Error() string {
	if err.Err == nil {
		return ""
	}
	return err.Err.Error()
}

func (err withContextError) Unwrap() error {
	return err.Err
}

// Merge will combine all given non nil error values into a single error value.
// If no valid error is given, nil is returned.
// If only a single non nil error value is given, the error value is returned.
func Merge(errs ...error) error {
	return errorkitlite.Merge(errs...)
}
