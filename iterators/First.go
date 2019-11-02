package iterators

import (
	"github.com/adamluzsi/frameless"
)

// First decode the first next value of the iterator and close the iterator
func First(i frameless.Iterator, ptr interface{}) (err error) {
	defer func() {
		cErr := i.Close()

		if err == nil {
			err = cErr
		}
	}()

	if !i.Next() {
		return frameless.ErrNotFound
	}

	if err := i.Decode(ptr); err != nil {
		return err
	}

	return i.Err()
}
