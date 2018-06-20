package iterators

import (
	"errors"
	"reflect"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless"
)

func NewForSlice(slice interface{}) frameless.Iterator {

	if reflect.TypeOf(slice).Kind() != reflect.Slice {
		panic("TypeError")
	}

	return &iterator{
		open:  true,
		rows:  reflect.ValueOf(slice),
		index: -1,
	}

}

type iterator struct {
	rows  reflect.Value
	open  bool
	index int
}

func (this *iterator) Close() error {
	this.open = false

	return nil
}

func (this *iterator) Err() error {
	return nil
}

func (this *iterator) Next() bool {
	if !this.open {
		return false
	}

	this.index++
	return this.rows.Len() > this.index
}

func (this *iterator) Decode(i interface{}) error {
	if !this.open {
		return errors.New("closed")
	}

	return reflects.Link(this.rows.Index(this.index).Interface(), i)
}
