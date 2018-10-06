package iterators

import (
	"github.com/adamluzsi/frameless"
)

var (
	// ErrClosed is the value that will be returned if a iterator has been closed but next decode is called
	ErrClosed frameless.Error = "Closed"
	// ErrNoNextElement defines that no next element in the iterator, used for iterateover exception cases
	ErrNoNextElement frameless.Error = "NoNextElement"
	// ErrUnexpectedNextElement is an error for the cases when it is an explicit requirement that only one element should be returned
	ErrUnexpectedNextElement frameless.Error = "ErrUnexpectedNextElement"
)
