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

func (this *Slice) Close() error {
	this.open = false

	return nil
}

func (this *Slice) Err() error {
	return nil
}

func (this *Slice) Next() bool {
	if !this.open {
		return false
	}

	this.index++
	return this.rows.Len() > this.index
}

func (this *Slice) Decode(i interface{}) error {
	if !this.open {
		return ErrClosed
	}

	return reflects.Link(this.rows.Index(this.index).Interface(), i)
}
