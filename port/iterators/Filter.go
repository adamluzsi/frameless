package iterators

func Filter[T any](i Iterator[T], filter func(T) bool) Iterator[T] {
	return &filterIter[T]{Iterator: i, Filter: filter}
}

type filterIter[T any] struct {
	Iterator Iterator[T]
	Filter   func(T) bool

	value T
}

func (i *filterIter[T]) Close() error {
	return i.Iterator.Close()
}

func (i *filterIter[T]) Err() error {
	return i.Iterator.Err()
}

func (i *filterIter[T]) Value() T {
	return i.value
}

func (i *filterIter[T]) Next() bool {
	if !i.Iterator.Next() {
		return false
	}
	i.value = i.Iterator.Value()
	if i.Filter(i.value) {
		return true
	}
	return i.Next()
}
