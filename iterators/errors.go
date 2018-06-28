package iterators

import "errors"

var (
	ErrClosed             = errors.New("closed")
	ErrNoNextElementFound = errors.New("no next element found")
)
