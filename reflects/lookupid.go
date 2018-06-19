package reflects

import (
	"reflect"
)

func LookupID(i interface{}) (string, bool) {

	r := reflect.ValueOf(i)
	val, ok := idReflectValue(r)

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
