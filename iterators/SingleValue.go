package iterators

// SingleValue creates an iterator that can return one single element and will ensure that Next can only be called once.
func SingleValue[T any](v T) *SingleValueIter[T] {
	return &SingleValueIter[T]{V: v}
}

type SingleValueIter[T any] struct {
	V T

	index  int
	closed bool
}

func (i *SingleValueIter[T]) Close() error {
	i.closed = true
	return nil
}

func (i *SingleValueIter[T]) Next() bool {
	if i.closed {
		return false
	}

	if i.index == 0 {
		i.index++
		return true
	}
	return false
}

func (i *SingleValueIter[T]) Err() error {
	return nil
}

func (i *SingleValueIter[T]) Value() T {
	return i.V
}
