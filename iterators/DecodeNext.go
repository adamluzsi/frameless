package iterators

import "github.com/adamluzsi/frameless/errs"

func DecodeNext(i Interface, e interface{}) error {

	if !i.Next() {
		return errs.ErrNotFound
	}

	return i.Decode(e)

}
