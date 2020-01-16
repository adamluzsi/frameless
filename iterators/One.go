package iterators

import "github.com/adamluzsi/frameless/errs"

func One(i Iterator, e interface{}) error {
	if err := i.Err(); err != nil {
		return err
	}

	if !i.Next() {
		return errs.ErrNotFound
	}

	if err := i.Decode(e); err != nil {
		return err
	}

	if i.Next() {
		return ErrUnexpectedNextElement
	}

	return i.Err()
}
