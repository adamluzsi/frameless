package iterators

// Error returns an Interface that only can do is returning an Err and never have next element
func Error[T any](err error) *ErrorIter[T] {
	return &ErrorIter[T]{err}
}

// ErrorIter iterator can be used for returning an error wrapped with iterator interface.
// This can be used when external resource encounter unexpected non recoverable error during query execution.
type ErrorIter[T any] struct {
	err error
}

func (i *ErrorIter[T]) Close() error {
	return nil
}

func (i *ErrorIter[T]) Next() bool {
	return false
}

func (i *ErrorIter[T]) Err() error {
	return i.err
}

func (i *ErrorIter[T]) Value() T {
	var v T
	return v
}
