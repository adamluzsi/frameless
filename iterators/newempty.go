package iterators

import (
	"github.com/adamluzsi/frameless"
)

// NewEmpty iterator is used to represent nil result with Null object pattern
func NewEmpty() frameless.Iterator {
	return &empty{}
}

type empty struct{}

func (i *empty) Close() error {
	return nil
}

func (i *empty) Next() bool {
	return false
}

func (i *empty) Err() error {
	return nil
}

func (i *empty) Decode(interface{}) error {
	return nil
}
