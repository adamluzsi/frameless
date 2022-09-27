package iterators

func Func[T any](next func() (v T, more bool, err error)) *FuncIter[T] {
	return &FuncIter[T]{NextFn: next}
}

type FuncIter[T any] struct {
	NextFn func() (v T, more bool, err error)

	value T
	err   error
}

func (i *FuncIter[T]) Close() error {
	return nil
}

func (i *FuncIter[T]) Err() error {
	return i.err
}

func (i *FuncIter[T]) Next() bool {
	value, more, err := i.NextFn()
	if err != nil {
		i.err = err
		return false
	}
	i.value = value
	return more
}

func (i *FuncIter[T]) Value() T {
	return i.value
}
