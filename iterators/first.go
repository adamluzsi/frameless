package iterators

import (
	"github.com/adamluzsi/frameless"
)

// First decode the first next value of the iterator and close the iterator
func First(i frameless.Iterator, e frameless.Entity) error {

	dErr := DecodeNext(i, e)
	cErr := i.Close()

	if dErr != nil {
		return dErr
	}

	if cErr != nil {
		return cErr
	}

	return nil

}
