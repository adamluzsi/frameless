package iterators

import (
	"reflect"
)

func Slice[T any](slice []T) *SliceIter[T] {
	if reflect.TypeOf(slice).Kind() != reflect.Slice {
		panic("TypeError")
	}

	return &SliceIter[T]{Slice: slice}
}

type SliceIter[T any] struct {
	Slice []T

	closed bool
	index  int
	value  T
}

func (i *SliceIter[T]) Close() error {
	i.closed = true
	return nil
}

func (i *SliceIter[T]) Err() error {
	return nil
}

func (i *SliceIter[T]) Next() bool {
	if i.closed {
		return false
	}

	if len(i.Slice) <= i.index {
		return false
	}

	i.value = i.Slice[i.index]
	i.index++
	return true
}

func (i *SliceIter[T]) Value() T {
	return i.value
}
