package iterators

import (
	"reflect"
)

func ForEach(i Interface, fn interface{}) (rErr error) {
	defer func() {
		cErr := i.Close()
		if rErr == nil {
			rErr = cErr
		}
	}()

	rfn := reflect.ValueOf(fn)

	for i.Next() {
		var v interface{}

		if err := i.Decode(&v); err != nil {
			return err
		}

		if err, ok := rfn.Call([]reflect.Value{reflect.ValueOf(v)})[0].Interface().(error); ok && err != nil {
			return err
		}
	}

	return i.Err()
}
