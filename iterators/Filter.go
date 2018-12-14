package iterators

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
)

func Filter(i frameless.Iterator, selector func(frameless.Entity) bool) *FilterIterator {
	return &FilterIterator{src: i, match: selector}
}

type FilterIterator struct {
	src   frameless.Iterator
	match func(frameless.Entity) bool

	next frameless.Entity
	err  error
}

func (fi *FilterIterator) Close() error {
	return fi.src.Close()
}

func (fi *FilterIterator) Err() error {
	if fi.err != nil {
		return fi.err
	}

	return fi.src.Err()
}

func (fi *FilterIterator) Decode(e frameless.Entity) error {
	return reflects.Link(fi.next, e)
}

func (fi *FilterIterator) Next() bool {

	hasNext := fi.src.Next()

	if !hasNext {
		return false
	}

	if err := fi.src.Decode(&fi.next); err != nil {
		fi.err = err
		return false
	}

	if fi.match(fi.next) {
		return true
	}

	return fi.Next()

}
