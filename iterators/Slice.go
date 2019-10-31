package iterators

import (
	"reflect"

	"github.com/adamluzsi/frameless/reflects"
)

func NewSlice(slice interface{}) *Slice {
	if reflect.TypeOf(slice).Kind() != reflect.Slice {
		panic("TypeError")
	}

	return &Slice{
		open:  true,
		rows:  reflect.ValueOf(slice),
		index: -1,
	}
}

type Slice struct {
	rows  reflect.Value
	open  bool
	index int
}

func (i *Slice) Close() error {
	i.open = false

	return nil
}

func (i *Slice) Err() error {
	return nil
}

func (i *Slice) Next() bool {
	if !i.open {
		return false
	}

	i.index++
	return i.rows.Len() > i.index
}

func (i *Slice) Decode(ptr interface{}) error {
	if !i.open {
		return ErrClosed
	}

	return reflects.Link(i.rows.Index(i.index).Interface(), ptr)
}
