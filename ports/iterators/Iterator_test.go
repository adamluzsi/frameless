package iterators_test

import (
	"go.llib.dev/frameless/ports/iterators"
)

func ExampleIterator() {
	var iter iterators.Iterator[int]
	defer iter.Close()
	for iter.Next() {
		v := iter.Value()
		_ = v
	}
	if err := iter.Err(); err != nil {
		// handle error
	}
}
