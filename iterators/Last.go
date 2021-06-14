package iterators

func Last(i Interface, e interface{}) (found bool, err error) {
	defer func() {
		cErr := i.Close()

		if err == nil && cErr != nil {
			err = cErr
		}
	}()

	iterated := false

	for i.Next() {
		iterated = true

		if err := i.Decode(e); err != nil {
			return false, err
		}
	}

	if !iterated {
		return false, i.Err()
	}

	if err := i.Err(); err != nil {
		return false, err
	}

	return true, nil
}
