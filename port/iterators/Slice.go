package iterators

func Slice[T any](slice []T) Iterator[T] {
	return &sliceIter[T]{Slice: slice}
}

type sliceIter[T any] struct {
	Slice []T

	closed bool
	index  int
	value  T
}

func (i *sliceIter[T]) Close() error {
	i.closed = true
	return nil
}

func (i *sliceIter[T]) Err() error {
	return nil
}

func (i *sliceIter[T]) Next() bool {
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

func (i *sliceIter[T]) Value() T {
	return i.value
}
