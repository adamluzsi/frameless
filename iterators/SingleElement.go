package iterators

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
)

// NewSingleElement creates an iterator that can return one single element and will ensure that Next can only be called once.
func NewSingleElement(e frameless.Entity) *SingleElement {
	return &SingleElement{element: e, index: -1, closed: false}
}

type SingleElement struct {
	element frameless.Entity
	index   int
	closed  bool
}

func (i *SingleElement) Close() error {
	i.closed = true
	return nil
}

func (i *SingleElement) Next() bool {
	i.index++

	return i.index == 0
}

func (i *SingleElement) Err() error {
	return nil
}

func (i *SingleElement) Decode(e frameless.Entity) error {

	if i.closed {
		return ErrClosed
	}

	if i.index == 0 {
		return reflects.Link(i.element, e)
	}

	return nil
}
