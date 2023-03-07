package iterators

// SingleValue creates an iterator that can return one single element and will ensure that Next can only be called once.
func SingleValue[T any](v T) Iterator[T] {
	return &singleValueIter[T]{V: v}
}

type singleValueIter[T any] struct {
	V T

	index  int
	closed bool
}

func (i *singleValueIter[T]) Close() error {
	i.closed = true
	return nil
}

func (i *singleValueIter[T]) Next() bool {
	if i.closed {
		return false
	}

	if i.index == 0 {
		i.index++
		return true
	}
	return false
}

func (i *singleValueIter[T]) Err() error {
	return nil
}

func (i *singleValueIter[T]) Value() T {
	return i.V
}
