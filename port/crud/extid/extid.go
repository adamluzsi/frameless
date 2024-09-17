package extid

import (
	"errors"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/zerokit"

	"go.llib.dev/frameless/pkg/reflectkit"
)

const errSetWithNonPtr errorkit.Error = "ptr should given as *ENT, else pass by value prevents the ID field remotely"

func Set[ID any](ptr any, id ID) error {
	if ptr == nil {
		return fmt.Errorf("nil given as ptr for extid.Set[%T]", *new(ID))
	}

	var (
		r  = reflect.ValueOf(ptr)
		rt = r.Type()
	)

	if !(rt.Kind() == reflect.Pointer && rt.Elem().Kind() != reflect.Pointer) {
		return errSetWithNonPtr
	}

	if r.IsNil() {
		return fmt.Errorf("nil pointer given for extid.Set[%T]", *new(ID))
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
	val := reflectkit.BaseValueOf(ent) // optimise this to use reflect.Value argument

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

func RegisterType[ENT, ID any](
	Get func(ENT) ID,
	Set func(*ENT, ID),
) any {
	register[reflect.TypeOf(*new(ENT))] = typeRegistration{
		Get: func(ent any) any {
			return Get(ent.(ENT))
		},
		Set: func(ptr any, id any) {
			Set(ptr.(*ENT), id.(ID))
		},
	}
	return nil
}

var register = map[reflect.Type]typeRegistration{}

type typeRegistration struct {
	Get func(any) any
	Set func(any, any)
}

// Accessor is a function that allows describing how to access an ID field in an ENT type.
// The returned id pointer will be used to Lookup its value, or to set new value to this ID pointer.
// Its functions will panic if func is provided, but it returns a nil pointer, as it is considered as implementation error.
//
// Example implementation:
//
//	extid.Accessor[Foo, FooID](func(v Foo) *FooID { return &v.ID })
//
// default: extid.Lookup, extid.Set, which will use either the `ext:"id"` tag, or the `ENT.ID()` & `ENT.SetID()` methods.
type Accessor[ENT, ID any] func(*ENT) *ID

func (fn Accessor[ENT, ID]) Lookup(ent ENT) (ID, bool) {
	if fn == nil {
		return Lookup[ID](ent)
	}
	id := fn.ptr(&ent)
	return *id, !zerokit.IsZero[ID](*id)
}

func (fn Accessor[ENT, ID]) Set(ent *ENT, id ID) error {
	if fn == nil {
		return Set(ent, id)
	}
	if ent == nil {
		return fmt.Errorf("nil %T pointer given for set %T", *new(ENT), *new(ID))
	}
	*fn.ptr(ent) = id
	return nil
}

func (fn Accessor[ENT, ID]) ptr(ent *ENT) *ID {
	if ent == nil {
		panic(fmt.Sprintf("nil %T error (%T)", *new(ENT), fn))
	}
	if fn == nil {
		panic("extid.MappingFunc implementation error")
	}
	id := fn(ent)
	if id == nil {
		var format string
		format = "implementation error: %T is provided, but it returned a nil pointer."
		format += "\nExample implementation: fn = func(v Foo) *FooID { return &v.ID }"
		panic(fmt.Sprintf(format, fn))
	}
	return id
}

type LookupIDFunc[ENT, ID any] func(ENT) (ID, bool)
