package iterators

import (
	"github.com/adamluzsi/frameless"
)

func Filter[T any](i frameless.Iterator[T], filter func(T) bool) *FilterIter[T] {
	return &FilterIter[T]{Iterator: i, Filter: filter}
}

type FilterIter[T any] struct {
	Iterator frameless.Iterator[T]
	Filter   func(T) bool

	value T
}

func (i *FilterIter[T]) Close() error {
	return i.Iterator.Close()
}

func (i *FilterIter[T]) Err() error {
	return i.Iterator.Err()
}

func (i *FilterIter[T]) Value() T {
	return i.value
}

func (i *FilterIter[T]) Next() bool {
	if !i.Iterator.Next() {
		return false
	}
	i.value = i.Iterator.Value()
	if i.Filter(i.value) {
		return true
	}
	return i.Next()
}
