package iterators_test

import (
	"github.com/adamluzsi/frameless/pkg/iterators"
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
