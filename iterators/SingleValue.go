package iterators

// NewSingleValue creates an iterator that can return one single element and will ensure that Next can only be called once.
func NewSingleValue[T any](v T) *SingleValue[T] {
	return &SingleValue[T]{V: v}
}

type SingleValue[T any] struct {
	V T

	index  int
	closed bool
}

func (i *SingleValue[T]) Close() error {
	i.closed = true
	return nil
}

func (i *SingleValue[T]) Next() bool {
	if i.closed {
		return false
	}

	if i.index == 0 {
		i.index++
		return true
	}
	return false
}

func (i *SingleValue[T]) Err() error {
	return nil
}

func (i *SingleValue[T]) Value() T {
	return i.V
}
