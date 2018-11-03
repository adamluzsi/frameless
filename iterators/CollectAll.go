package iterators

import (
	"reflect"

	"github.com/adamluzsi/frameless"
)

func CollectAll(i frameless.Iterator, PointerToTheSlice interface{}) error {
	defer i.Close()

	ptr := reflect.ValueOf(PointerToTheSlice)

	slice := ptr.Elem()

	elemBaseType := slice.Type().Elem()

	for elemBaseType.Kind() == reflect.Ptr {
		elemBaseType = elemBaseType.Elem()
	}

	values := []reflect.Value{}

	for i.Next() {
		elem := reflect.New(elemBaseType)

		if err := i.Decode(elem.Interface()); err != nil {
			return err
		}

		if slice.Type().Elem().Kind() == reflect.Ptr {
			values = append(values, elem)
		} else {
			values = append(values, elem.Elem())
		}
	}

	slice.Set(reflect.Append(slice, values...))

	return i.Err()
}
