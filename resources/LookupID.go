package resources

import (
	"github.com/adamluzsi/frameless/reflects"
	"reflect"
)

func LookupID(i interface{}) (string, bool) {

	_, val, ok := idReflectValue(reflects.BaseValueOf(i))

	if ok {
		return val.String(), true
	}

	return "", false

}

func idReflectValue(val reflect.Value) (reflect.StructField, reflect.Value, bool) {

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	sf, byTag, ok := lookupByTag(val)

	if ok {
		return sf, byTag, true
	}

	const name = `ID`

	byName := val.FieldByName(name)

	if byName.Kind() != reflect.Invalid {
		sf, _ := val.Type().FieldByName(name)
		return sf, byName, true
	}

	return reflect.StructField{}, reflect.Value{}, false

}

func lookupByTag(val reflect.Value) (reflect.StructField, reflect.Value, bool) {

	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		structField := val.Type().Field(i)
		tag := structField.Tag

		if tag.Get("ext") == "ID" {
			return structField, valueField, true
		}
	}

	return reflect.StructField{}, reflect.Value{}, false

}
