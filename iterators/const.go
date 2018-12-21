package iterators

import (
	"github.com/adamluzsi/frameless"
)

const (
	// ErrClosed is the value that will be returned if a iterator has been closed but next decode is called
	ErrClosed frameless.Error = "Closed"
	// ErrUnexpectedNextElement is an error for the cases when it is an explicit requirement that only one element should be returned
	ErrUnexpectedNextElement frameless.Error = "ErrUnexpectedNextElement"
)
