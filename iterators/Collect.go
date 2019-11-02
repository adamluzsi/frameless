package iterators

import (
	"reflect"

	"github.com/adamluzsi/frameless"
)

func Collect(i frameless.Iterator, slicePtr interface{}) (err error) {
	defer func() {
		closeErr := i.Close()
		if err == nil {
			err = closeErr
		}
	}()

	ptr := reflect.ValueOf(slicePtr)
	slice := ptr.Elem()
	sliceElementType := slice.Type().Elem()

	var values []reflect.Value

	for i.Next() {
		elem := reflect.New(sliceElementType)

		if err := i.Decode(elem.Interface()); err != nil {
			return err
		}

		values = append(values, elem.Elem())
	}

	slice.Set(reflect.Append(slice, values...))

	return i.Err()
}
