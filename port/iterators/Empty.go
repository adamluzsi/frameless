package iterators

// Empty iterator is used to represent nil result with Null object pattern
func Empty[T any]() Iterator[T] {
	return &emptyIter[T]{}
}

// emptyIter iterator can help achieve Null Object Pattern when no value is logically expected and iterator should be returned
type emptyIter[T any] struct{}

func (i *emptyIter[T]) Close() error {
	return nil
}

func (i *emptyIter[T]) Next() bool {
	return false
}

func (i *emptyIter[T]) Err() error {
	return nil
}

func (i *emptyIter[T]) Value() T {
	var v T
	return v
}
