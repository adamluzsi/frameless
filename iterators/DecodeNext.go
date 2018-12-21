package iterators

import (
	"github.com/adamluzsi/frameless"
)

func DecodeNext(i frameless.Iterator, e frameless.Entity) error {

	if !i.Next() {
		return frameless.ErrNotFound
	}

	return i.Decode(e)

}
