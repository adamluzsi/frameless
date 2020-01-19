package iterators

import (
	"github.com/adamluzsi/frameless/errs"
)

const (
	ErrNotFound errs.Error = errs.ErrNotFound
	// ErrClosed is the value that will be returned if a iterator has been closed but next decode is called
	ErrClosed errs.Error = "Closed"
	// ErrUnexpectedNextElement is an error for the cases when it is an explicit requirement that only one element should be returned
	ErrUnexpectedNextElement errs.Error = "ErrUnexpectedNextElement"
)
