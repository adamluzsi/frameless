package iterators

import (
	"errors"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
)

// NewForStruct creates an iterator that can return one single element and will ensure that Next can only be called once.
func NewForStruct(e frameless.Entity) frameless.Iterator {
	return &singleElementIterator{element: e, index: -1, closed: false}
}

type singleElementIterator struct {
	element frameless.Entity
	index   int
	closed  bool
}

func (i *singleElementIterator) Close() error {
	i.closed = true
	return nil
}

func (i *singleElementIterator) Next() bool {
	i.index++

	return i.index == 0
}

func (i *singleElementIterator) Err() error {
	return nil
}

func (i *singleElementIterator) Decode(e frameless.Entity) error {

	if i.closed {
		return errors.New("closed")
	}

	if i.index == 0 {
		return reflects.Link(i.element, e)
	}

	return nil
}
