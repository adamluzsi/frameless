package iterators

// NewError returns an Interface that only can do is returning an Err and never have next element
func NewError[T any](err error) *Error[T] {
	return &Error[T]{err}
}

// Error iterator can be used for returning an error wrapped with iterator interface.
// This can be used when external resource encounter unexpected non recoverable error during query execution.
type Error[T any] struct {
	err error
}

func (i *Error[T]) Close() error {
	return nil
}

func (i *Error[T]) Next() bool {
	return false
}

func (i *Error[T]) Err() error {
	return i.err
}

func (i *Error[T]) Value() T {
	var v T
	return v
}
