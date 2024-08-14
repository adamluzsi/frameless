package iterators

func Last[T any](i Iterator[T]) (value T, found bool, err error) {
	defer func() {
		cErr := i.Close()
		if err == nil && cErr != nil {
			err = cErr
		}
	}()
	iterated := false
	var v T
	for i.Next() {
		iterated = true
		v = i.Value()
	}
	if err := i.Err(); err != nil {
		return v, false, err
	}
	if !iterated {
		return v, false, nil
	}
	return v, true, nil
}
