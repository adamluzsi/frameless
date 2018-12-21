package iterators

import (
	"github.com/adamluzsi/frameless"
)

// First decode the first next value of the iterator and close the iterator
func First(i frameless.Iterator, e frameless.Entity) (err error) {

	defer func() {
		cErr := i.Close()

		if err == nil && cErr != nil {
			err = cErr
		}
	}()

	if !i.Next() {
		return frameless.ErrNotFound
	}

	if err := i.Decode(e); err != nil {
		return err
	}

	return i.Err()
}
