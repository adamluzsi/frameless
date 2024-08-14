package iterators

// Error returns an Interface that only can do is returning an Err and never have next element
func Error[T any](err error) Iterator[T] {
	return &errorIter[T]{err}
}

// errorIter iterator can be used for returning an error wrapped with iterator interface.
// This can be used when external resource encounter unexpected non recoverable error during query execution.
type errorIter[T any] struct {
	err error
}

func (i *errorIter[T]) Close() error {
	return nil
}

func (i *errorIter[T]) Next() bool {
	return false
}

func (i *errorIter[T]) Err() error {
	return i.err
}

func (i *errorIter[T]) Value() T {
	var v T
	return v
}
