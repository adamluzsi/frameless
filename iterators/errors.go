package iterators

import "errors"

var (
	// ErrClosed is the value that will be returned if a iterator has been closed but next decode is called
	ErrClosed = errors.New("Closed")
	// ErrNoNextElement defines that no next element in the iterator, used for iterateover exception cases
	ErrNoNextElement = errors.New("NoNextElement")
)
