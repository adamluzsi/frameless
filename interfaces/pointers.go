package interfaces

import "reflect"

func ReplaceValue(with, what interface{}) {
	from := reflect.ValueOf(with)
	to := reflect.ValueOf(what)

	if from.Kind() == reflect.Ptr {
		to.Elem().Set(from.Elem())
	} else {
		to.Elem().Set(from)
	}
}
