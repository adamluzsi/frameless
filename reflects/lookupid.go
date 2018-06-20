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

func lookupByTag(val reflect.Value) (reflect.Value, bool) {

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		if tag.Get("frameless") == "ID" {
			return valueField, true
		}
	}

	return val, false
}
