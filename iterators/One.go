package iterators

import (
	"github.com/adamluzsi/frameless"
)

func One(i frameless.Iterator, e frameless.Entity) error {
	if err := i.Err(); err != nil {
		return err
	}

	if !i.Next() {
		return frameless.ErrNotFound
	}

	if err := i.Decode(e); err != nil {
		return err
	}

	if i.Next() {
		return ErrUnexpectedNextElement
	}

	return i.Err()
}
