package reflects

import (
	"reflect"
)

func LookupID(i interface{}) (string, bool) {

	val, ok := idReflectValue(reflect.ValueOf(i))

	if ok {
		return val.String(), true
	}

	return "", false
}
