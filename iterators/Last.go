package iterators

import (
	"github.com/adamluzsi/frameless"
)

func Last(i frameless.Iterator, e frameless.Entity) (err error) {

	defer func() {
		cErr := i.Close()

		if err == nil && cErr != nil {
			err = cErr
		}
	}()

	iterated := false

	for i.Next() {

		iterated = true

		if err := i.Decode(e); err != nil {
			return err
		}
	}

	if !iterated {
		return frameless.ErrNotFound
	}

	return i.Err()

}
