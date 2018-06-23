package iterators

import (
	"github.com/adamluzsi/frameless"
)

func NewEmpty() frameless.Iterator {
	return &newEmpty{}
}

type newEmpty struct{}

func (i *newEmpty) Close() error {
	return nil
}

func (i *newEmpty) Next() bool {
	return false
}

func (i *newEmpty) Err() error {
	return nil
}

func (i *newEmpty) Decode(interface{}) error {
	return nil
}
