package iterators

// First decode the first next value of the iterator and close the iterator
func First(i Interface, ptr interface{}) (err error) {
	defer func() {
		cErr := i.Close()

		if err == nil {
			err = cErr
		}
	}()

	if !i.Next() {
		return ErrNotFound
	}

	if err := i.Decode(ptr); err != nil {
		return err
	}

	return i.Err()
}
