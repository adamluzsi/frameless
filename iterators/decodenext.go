package iterators

import (
	"errors"

	"github.com/adamluzsi/frameless"
)

func DecodeNext(i frameless.Iterator, e frameless.Entity) error {

	if !i.Next() {
		return errors.New("no next element found")
	}

	return i.Decode(e)

}
