package iterators

import "github.com/adamluzsi/frameless/errs"

func DecodeNext(i Iterator, e interface{}) error {

	if !i.Next() {
		return errs.ErrNotFound
	}

	return i.Decode(e)

}
