package frameless_test

import "github.com/adamluzsi/frameless"

func ExampleIterator() {
	var iter frameless.Iterator[int]
	defer iter.Close()
	for iter.Next() {
		v := iter.Value()
		_ = v
	}
	if err := iter.Err(); err != nil {
		// handle error
	}
}
