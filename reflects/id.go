package reflects

import (
	"reflect"
)

func LookupID(i interface{}) (string, bool) {

	r := reflect.ValueOf(i)
	if r.Kind() == reflect.Ptr {
		r = r.Elem()
	}

	byName := r.FieldByName("ID")
	if byName.Kind() != reflect.Invalid {
		return byName.String(), true
	}

	byTag, ok := lookupByTag(r)
	if ok {
		return byTag.String(), true
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
