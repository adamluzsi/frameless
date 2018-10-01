package iterators

import "github.com/adamluzsi/frameless/errtype"

var (
	// ErrClosed is the value that will be returned if a iterator has been closed but next decode is called
	ErrClosed errtype.Error = "Closed"
	// ErrNoNextElement defines that no next element in the iterator, used for iterateover exception cases
	ErrNoNextElement errtype.Error = "NoNextElement"
)
