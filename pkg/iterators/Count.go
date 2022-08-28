package iterators

// Count will iterate over and count the total iterations number
//
// Good when all you want is count all the elements in an iterator but don't want to do anything else.
func Count[T any](i Iterator[T]) (total int, err error) {
	defer func() {
		closeErr := i.Close()
		if err == nil {
			err = closeErr
		}
	}()
	total = 0
	for i.Next() {
		total++
	}
	return total, i.Err()
}
