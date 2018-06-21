package iterators

import (
	"github.com/adamluzsi/frameless"
)

// NewForError returns an Iterator that only can do is returning an Err and never have next element
func NewForError(err error) frameless.Iterator {
	return &errIterator{err}
}

type errIterator struct {
	err error
}

func (i *errIterator) Close() error {
	return nil
}

func (i *errIterator) Next() bool {
	return false
}

func (i *errIterator) Err() error {
	return i.err
}

func (i *errIterator) Decode(interface{}) error {
	return nil
}
