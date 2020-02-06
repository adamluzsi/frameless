package iterators

// First decode the first next value of the iterator and close the iterator
func First(i Interface, ptr interface{}) (found bool, err error) {
	defer func() {
		cErr := i.Close()

		if err == nil {
			err = cErr
		}
	}()

	if !i.Next() {
		return false, nil
	}

	if err := i.Decode(ptr); err != nil {
		return false, err
	}

	if err := i.Err(); err != nil {
		return false, err
	}

	return true, nil
}
