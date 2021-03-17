package iterators

import (
	"reflect"
)

func Collect(i Interface, slicePtr interface{}) (err error) {
	defer func() {
		closeErr := i.Close()
		if err == nil {
			err = closeErr
		}
	}()

	var (
		ptr       = reflect.ValueOf(slicePtr)
		sliceType = reflect.TypeOf(ptr.Elem().Interface())
		elemType  = sliceType.Elem()
	)

	slice := reflect.MakeSlice(sliceType, 0, 0)
	for i.Next() {
		elem := reflect.New(elemType)
		if err := i.Decode(elem.Interface()); err != nil {
			return err
		}

		slice = reflect.Append(slice, elem.Elem())
	}

	ptr.Elem().Set(slice)
	return i.Err()
}
