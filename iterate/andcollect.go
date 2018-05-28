package iterate

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/adamluzsi/frameless"
)

func AndCollect(slice interface{}, iterator frameless.Iterator) error {
	if v := reflect.ValueOf(slice); v.Kind() != reflect.Slice {
		return errors.New(fmt.Sprintf("slice type expected but %s given", v.Kind().String()))
	}

	itemTyp := reflect.TypeOf(slice).Elem()
	item := reflect.New(itemTyp)

	item.Elem().FieldByName("Id").SetInt(1)

	for iterator.More() {
		iterator.Decode(item.Interface())
	}

	return iterator.Err()
}
