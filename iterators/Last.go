package iterators

import "github.com/adamluzsi/frameless/errs"

func Last(i Interface, e interface{}) (err error) {

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
		return errs.ErrNotFound
	}

	return i.Err()

}
