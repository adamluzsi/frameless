package iterators

// First decode the first next value of the iterator and close the iterator
func First[T any](i Iterator[T]) (value T, found bool, err error) {
	defer func() {
		cErr := i.Close()
		if err == nil {
			err = cErr
		}
	}()
	if !i.Next() {
		return value, false, i.Err()
	}
	return i.Value(), true, i.Err()
}
