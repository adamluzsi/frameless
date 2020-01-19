package iterators

func DecodeNext(i Interface, e interface{}) error {

	if !i.Next() {
		return ErrNotFound
	}

	return i.Decode(e)

}
