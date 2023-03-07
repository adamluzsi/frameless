package iterators

func Func[T any](next func() (v T, more bool, err error)) Iterator[T] {
	return &funcIter[T]{NextFn: next}
}

type funcIter[T any] struct {
	NextFn func() (v T, more bool, err error)

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
	value, more, err := i.NextFn()
	if err != nil {
		i.err = err
		return false
	}
	i.value = value
	return more
}

func (i *funcIter[T]) Value() T {
	return i.value
}
