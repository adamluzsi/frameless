package iterators

// Count will iterate over and count the total iterations number
//
// Good when all you want is count all the elements in an iterator but don't want to do anything else.
func Count(i Interface) (int, error) {
	defer i.Close()

	total := 0

	for i.Next() {
		total++
	}

	return total, i.Err()
}
