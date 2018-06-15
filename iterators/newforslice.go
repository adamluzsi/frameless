package iterators

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/adamluzsi/frameless"
)

func test(t interface{}) {
	switch reflect.TypeOf(t).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(t)

		for i := 0; i < s.Len(); i++ {
			fmt.Println(s.Index(i))
		}
	}
}

func NewForSlice(slice interface{}) frameless.Iterator {

	if reflect.TypeOf(slice).Kind() != reflect.Slice {
		panic("invalid type")
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

	value := this.rows.Index(this.index)
	dest := reflect.ValueOf(i)
	dest.Elem().Set(value)

	return nil
}
