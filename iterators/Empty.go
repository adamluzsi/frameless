package iterators

// Empty iterator is used to represent nil result with Null object pattern
func Empty[T any]() *EmptyIter[T] {
	return &EmptyIter[T]{}
}

// EmptyIter iterator can help achieve Null Object Pattern when no value is logically expected and iterator should be returned
type EmptyIter[T any] struct{}

func (i *EmptyIter[T]) Close() error {
	return nil
}

func (i *EmptyIter[T]) Next() bool {
	return false
}

func (i *EmptyIter[T]) Err() error {
	return nil
}

func (i *EmptyIter[T]) Value() T {
	var v T
	return v
}
