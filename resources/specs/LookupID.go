package specs

import (
	"github.com/adamluzsi/frameless/reflects"
	"reflect"
)

func LookupID(i interface{}) (string, bool) {

	val, ok := idReflectValue(reflects.BaseValueOf(i))

	if ok {
		return val.String(), true
	}

	return "", false
}

func idReflectValue(val reflect.Value) (reflect.Value, bool) {

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	byTag, ok := lookupByTag(val)
	if ok {
		return byTag, true
	}

	byName := val.FieldByName("ID")
	if byName.Kind() != reflect.Invalid {
		return byName, true
	}

	return reflect.Value{}, false

}

func lookupByTag(val reflect.Value) (reflect.Value, bool) {

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		if tag.Get("ext") == "ID" {
			return valueField, true
		}
	}

	return val, false
}
