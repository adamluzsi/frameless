package resources

import (
	"errors"
	"reflect"

	"github.com/adamluzsi/frameless/reflects"
)

func SetID(ptr interface{}, id interface{}) error {
	r := reflect.ValueOf(ptr)

	if r.Kind() != reflect.Ptr {
		return errors.New("ptr should be given, else Pass By Value prevent setting struct ID field remotely")
	}

	_, val, ok := LookupIDStructField(ptr)

	if !ok {
		return errors.New("could not locate ID field in the given structure")
	}

	val.Set(reflect.ValueOf(id))

	return nil
}

func LookupID(i interface{}) (id interface{}, ok bool) {
	_, val, ok := LookupIDStructField(i)

	if !ok {
		return nil, false
	}

	return val.Interface(), !isNil(val)
}

func isNil(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Interface:
		return isNil(val.Elem())

	case reflect.Ptr, reflect.Slice, reflect.Chan, reflect.Func, reflect.Map:
		return val.IsNil()

	default:
		return !val.IsValid() || val.IsZero()

	}
}

func LookupIDStructField(ent interface{}) (reflect.StructField, reflect.Value, bool) {
	val := reflects.BaseValueOf(ent)

	sf, byTag, ok := lookupByTag(val)
	if ok {
		return sf, byTag, true
	}

	const name = `ID`
	if byName := val.FieldByName(name); byName.Kind() != reflect.Invalid {
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
