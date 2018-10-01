package storages

import (
	"reflect"
)

func idReflectValue(val reflect.Value) (reflect.Value, bool) {

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	byName := val.FieldByName("ID")
	if byName.Kind() != reflect.Invalid {
		return byName, true
	}

	// TODO specific this to be the first expected use case
	byTag, ok := lookupByTag(val)
	if ok {
		return byTag, true
	}

	return reflect.Value{}, false

}

func lookupByTag(val reflect.Value) (reflect.Value, bool) {

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		if tag.Get("storage") == "ID" {
			return valueField, true
		}
	}

	return val, false
}
