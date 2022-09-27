package extid

import (
	"errors"
	"reflect"

	"github.com/adamluzsi/frameless/pkg/reflects"
)

func Set(ptr interface{}, id interface{}) error {
	r := reflect.ValueOf(ptr)

	if r.Kind() != reflect.Ptr {
		return errors.New("ptr should be given, else Pass By Value prevent setting struct ID field remotely")
	}

	_, val, ok := LookupStructField(ptr)

	if !ok {
		return errors.New("could not locate ID field in the given structure")
	}

	val.Set(reflect.ValueOf(id))

	return nil
}

func Lookup[ID any](i interface{}) (id ID, ok bool) {
	_, val, ok := LookupStructField(i)
	if !ok {
		return id, false
	}

	id, ok = val.Interface().(ID)
	if !ok {
		return id, false
	}
	if isEmpty(id) {
		return id, false
	}
	return id, ok
}

func isEmpty(i interface{}) (ok bool) {
	rv := reflect.ValueOf(i)
	defer func() {
		if v := recover(); v == nil {
			return
		}
		ok = rv.IsZero()
	}()
	return rv.IsNil()
}

func LookupStructField(ent interface{}) (reflect.StructField, reflect.Value, bool) {
	val := reflects.BaseValueOf(ent)

	sf, byTag, ok := lookupByTag(val)
	if ok {
		return sf, byTag, true
	}

	const upper = `ID`
	if byName := val.FieldByName(upper); byName.Kind() != reflect.Invalid {
		sf, _ := val.Type().FieldByName(upper)
		return sf, byName, true
	}

	return reflect.StructField{}, reflect.Value{}, false
}

func lookupByTag(val reflect.Value) (reflect.StructField, reflect.Value, bool) {
	const (
		lower = "id"
		upper = "ID"
	)
	for i := 0; i < val.NumField(); i++ {
		valueField := val.Field(i)
		structField := val.Type().Field(i)
		tag := structField.Tag

		if tagValue := tag.Get("ext"); tagValue == upper || tagValue == lower {
			return structField, valueField, true
		}
	}

	return reflect.StructField{}, reflect.Value{}, false

}
