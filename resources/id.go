package resources

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/adamluzsi/frameless/reflects"
)

func SetID(ptr interface{}, id interface{}) error {
	r := reflect.ValueOf(ptr)

	if r.Kind() != reflect.Ptr {
		return errors.New("ptr should be given, else Pass By Value prevent setting struct ID field remotely")
	}

	_, val, ok := idReflectValue(r)

	if !ok {
		return errors.New("could not locate ID field in the given structure")
	}

	val.Set(reflect.ValueOf(id))

	return nil
}

func LookupID(i interface{}) (id interface{}, ok bool) {
	_, val, ok := idReflectValue(reflects.BaseValueOf(i))

	if !ok {
		return nil, false
	}

	return val.Interface(), !isNil(val)
}

func isNil(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Interface:
		fmt.Println(`?`)
		return isNil(val.Elem())

	case reflect.Ptr, reflect.Slice, reflect.Chan, reflect.Func, reflect.Map:
		fmt.Println(`??`)
		return val.IsNil()

	default:
		return !val.IsValid() || val.IsZero()

	}
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
