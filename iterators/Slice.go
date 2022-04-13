package iterators

import (
	"reflect"
)

func NewSlice[T any](slice []T) *Slice[T] {
	if reflect.TypeOf(slice).Kind() != reflect.Slice {
		panic("TypeError")
	}

	return &Slice[T]{Slice: slice}
}

type Slice[T any] struct {
	Slice []T

	closed bool
	index  int
	value  T
}

func (i *Slice[T]) Close() error {
	i.closed = true
	return nil
}

func (i *Slice[T]) Err() error {
	return nil
}

func (i *Slice[T]) Next() bool {
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

func (i *Slice[T]) Value() T {
	return i.value
}
