package errorkit

import (
	"errors"
	"fmt"
)

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
	return wrapper{Owner: err, Wrapped: oth}
}

// F will format the error value
func (err Error) F(format string, a ...any) error { return err.Wrap(fmt.Errorf(format, a...)) }

type wrapper struct {
	Owner   Error
	Wrapped error // must be not nil
}

func (w wrapper) Error() string {
	return fmt.Sprintf("[%s] %s", w.Owner, w.Wrapped.Error())
}

func (w wrapper) As(target any) bool {
	return errors.As(w.Owner, target) || errors.As(w.Wrapped, target)
}

func (w wrapper) Is(target error) bool {
	return errors.Is(w.Owner, target) || errors.Is(w.Wrapped, target)
}
