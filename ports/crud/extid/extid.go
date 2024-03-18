package extid

import (
	"errors"
	"go.llib.dev/frameless/pkg/errorkit"
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit"
)

const errSetWithNonPtr errorkit.Error = "ptr should given as *Entity, else pass by value prevents the ID field remotely"

func Set[ID any](ptr any, id ID) error {
	var (
		r  = reflect.ValueOf(ptr)
		rt = r.Type()
	)

	if !(rt.Kind() == reflect.Ptr && rt.Elem().Kind() != reflect.Ptr) {
		return errSetWithNonPtr
	}

	tr, ok := register[rt.Elem()]
	if ok {
		tr.Set(ptr, id)
		return nil
	}

	_, val, ok := lookupStructField(ptr)
	if !ok {
		return errors.New("could not locate ID field in the given structure")
	}

	val.Set(reflect.ValueOf(id))

	return nil
}

func Lookup[ID, Ent any](ent Ent) (id ID, ok bool) {
	if tr, ok := register[reflectkit.BaseValueOf(ent).Type()]; ok {
		return tr.Get(ent).(ID), true
	}

	_, val, ok := lookupStructField(ent)
	if !ok {
		return id, false
	}

	id, ok = val.Interface().(ID)
	if !ok {
		return id, false
	}
	if isEmpty(id) { // TODO: this doesn't feel right as ok should mean the ID is found, not that id is empty
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

func lookupStructField(ent interface{}) (reflect.StructField, reflect.Value, bool) {
	val := reflectkit.BaseValueOf(ent)

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

//--------------------------------------------------------------------------------------------------------------------//

func RegisterType[Entity, ID any](
	Get func(Entity) ID,
	Set func(*Entity, ID),
) any {
	register[reflect.TypeOf(*new(Entity))] = typeRegistration{
		Get: func(ent any) any {
			return Get(ent.(Entity))
		},
		Set: func(ptr any, id any) {
			Set(ptr.(*Entity), id.(ID))
		},
	}
	return nil
}

var register = map[reflect.Type]typeRegistration{}

type typeRegistration struct {
	Get func(any) any
	Set func(any, any)
}
