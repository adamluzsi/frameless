package iterators

// Func enables you to create an iterator with a lambda expression.
// Func is very useful when you have to deal with non type safe iterators
// that you would like to map into a type safe variant.
// In case you need to close the currently mapped resource, use the OnClose callback option.
func Func[T any](next func() (v T, ok bool, err error), callbackOptions ...CallbackOption) Iterator[T] {
	var iter Iterator[T]
	iter = &funcIter[T]{NextFn: next}
	iter = WithCallback(iter, callbackOptions...)
	return iter
}

type funcIter[T any] struct {
	NextFn func() (v T, ok bool, err error)

	value T
	err   error
}

func (i *funcIter[T]) Close() error {
	return nil
}

func (i *funcIter[T]) Err() error {
	return i.err
}

func (i *funcIter[T]) Next() bool {
	if i.err != nil {
		return false
	}
	value, ok, err := i.NextFn()
	if err != nil {
		i.err = err
		return false
	}
	if !ok {
		return false
	}
	i.value = value
	return true
}

func (i *funcIter[T]) Value() T {
	return i.value
}
