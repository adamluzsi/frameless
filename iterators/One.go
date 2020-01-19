package iterators

func One(i Interface, e interface{}) error {
	if err := i.Err(); err != nil {
		return err
	}

	if !i.Next() {
		return ErrNotFound
	}

	if err := i.Decode(e); err != nil {
		return err
	}

	if i.Next() {
		return ErrUnexpectedNextElement
	}

	return i.Err()
}
