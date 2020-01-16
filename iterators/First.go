package iterators

import "github.com/adamluzsi/frameless/errs"

// First decode the first next value of the iterator and close the iterator
func First(i Iterator, ptr interface{}) (err error) {
	defer func() {
		cErr := i.Close()

		if err == nil {
			err = cErr
		}
	}()

	if !i.Next() {
		return errs.ErrNotFound
	}

	if err := i.Decode(ptr); err != nil {
		return err
	}

	return i.Err()
}
