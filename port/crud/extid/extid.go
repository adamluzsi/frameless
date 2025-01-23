package extid

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/zerokit"
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

	_, val, ok := ExtractIdentifierField(ptr)
	if !ok {
		return errors.New("could not locate ID field in the given structure")
	}

	val.Set(reflect.ValueOf(id))

	return nil
}

func Lookup[ID, Ent any](ent Ent) (id ID, ok bool) {
	val := reflectkit.BaseValueOf(ent)
	if tr, ok := register[val.Type()]; ok {
		id := tr.Get(val.Interface()).(ID)
		return id, !zerokit.IsZero(id)
	}
	_, val, ok = ExtractIdentifierField(val)
	if !ok {
		return id, false
	}
	if reflectkit.IsEmpty(val) {
		return id, false
	}
	id, ok = val.Interface().(ID)
	if !ok {
		return id, false
	}
	return id, ok
}

var cacheExtractIdentifierField synckit.Map[reflect.Type, func(reflect.Value) (reflect.StructField, reflect.Value, bool)]

func ExtractIdentifierField(ent any) (reflect.StructField, reflect.Value, bool) {
	val := reflectkit.ToValue(ent)
	val = reflectkit.BaseValue(val)
	init := func() func(reflect.Value) (reflect.StructField, reflect.Value, bool) {
		return refMakeExtractFunc(val)
	}
	return cacheExtractIdentifierField.GetOrInit(val.Type(), init)(val)
}

func refMakeExtractFunc(val reflect.Value) func(reflect.Value) (reflect.StructField, reflect.Value, bool) {
	{
		if val.Kind() != reflect.Struct {
			return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
				return reflect.StructField{}, reflect.Value{}, false
			}
		}
	}
	{ // lookup by "ext":"id" tag
		const extTagIDFlag = "id"
		for i := 0; i < val.NumField(); i++ {
			sf := val.Type().Field(i)
			tagValue := sf.Tag.Get("ext")
			if strings.EqualFold(tagValue, extTagIDFlag) {
				index := i
				return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
					return sf, v.Field(index), true
				}
			}
		}
	}
	{ // lookup by ID field
		const structIDFieldName = `ID`
		byName := val.FieldByName(structIDFieldName)
		if byName.Kind() != reflect.Invalid {
			sf, ok := val.Type().FieldByName(structIDFieldName)
			if ok && len(sf.Index) == 1 {
				return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
					return sf, v.FieldByIndex(sf.Index), true
				}
			}
		}
	}
	{ // lookup ID in the first embeded field
		var T = val.Type()
		var fieldNumber = T.NumField()
		for i := 0; i < fieldNumber; i++ {
			var (
				i  = i
				sf = T.Field(i)
			)

			if !sf.Anonymous /* not embedded */ {
				continue
			}

			field := val.FieldByIndex(sf.Index)
			extractFunc := refMakeExtractFunc(field)
			if _, _, ok := extractFunc(field); ok {
				return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
					return ExtractIdentifierField(v.FieldByIndex(sf.Index))
				}
			}
		}
	}
	return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
		return reflect.StructField{}, reflect.Value{}, false
	}
}

//--------------------------------------------------------------------------------------------------------------------//

func RegisterType[ENT, ID any](
	Get func(ENT) ID,
	Set func(*ENT, ID),
) func() {
	key := reflectkit.TypeOf[ENT]()
	register[key] = typeRegistration{
		Get: func(ent any) any {
			return Get(ent.(ENT))
		},
		Set: func(ptr any, id any) {
			Set(ptr.(*ENT), id.(ID))
		},
	}
	return func() {
		delete(register, key)
	}
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

func (fn Accessor[ENT, ID]) ReflectLookup(rENT reflect.Value) (rID reflect.Value, ok bool) {
	if rENT.Type() != reflectkit.TypeOf[ENT]() {
		return reflect.Value{}, false
	}
	if reflectkit.IsZero(rENT) {
		return reflect.Value{}, false
	}
	id, ok := fn.Lookup(rENT.Interface().(ENT))
	return reflect.ValueOf(id), ok
}

func (fn Accessor[ENT, ID]) ReflectSet(ptrENT reflect.Value, id reflect.Value) error {
	var (
		expPtrType = reflectkit.TypeOf[*ENT]()
		expIDType  = reflectkit.TypeOf[ID]()
	)
	if ptrENT.Type() != expPtrType {
		return fmt.Errorf("extid.Accessor#ReflectSet type mismatch for *ENT, expected %s but got %s",
			expPtrType.String(), ptrENT.Type().String())
	}
	if id.Type() != expIDType {
		return fmt.Errorf("extid.Accessor#ReflectSet type mismatch for ID, expected %s but got %s",
			expIDType.String(), id.Type().String())
	}
	return fn.Set((*ENT)(ptrENT.UnsafePointer()), id.Interface().(ID))
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

type ReflectAccessor func(ptrENT reflect.Value) (ptrID reflect.Value)

func (fn ReflectAccessor) ReflectLookup(rENT reflect.Value) (rID reflect.Value, ok bool) {
	defer func() { recover() }()
	ptrID := fn(reflectkit.PointerOf(rENT))
	fn.checkPtrID(ptrID)
	id := ptrID.Elem()
	return id, !reflectkit.IsEmpty(id)
}

func (fn ReflectAccessor) ReflectSet(ptrENT reflect.Value, id reflect.Value) (rErr error) {
	defer errorkit.Recover(&rErr)
	if ptrENT.Kind() != reflect.Pointer {
		return fmt.Errorf("%w: pointer ENT type was expected", reflectkit.ErrTypeMismatch)
	}
	if ptrENT.IsNil() {
		return fmt.Errorf("%w: nil ENT pointer is given", reflectkit.ErrTypeMismatch)
	}
	ptrID := fn(ptrENT)
	fn.checkPtrID(ptrID)
	if expIDType := ptrID.Type().Elem(); id.Type() != expIDType {
		return fmt.Errorf("%w: ReflectAccessor#ReflectSet expected %s ID type, but got %s", reflectkit.ErrTypeMismatch,
			expIDType.String(), id.Type().String())
	}
	ptrID.Elem().Set(id)
	return nil
}

func (fn ReflectAccessor) checkPtrID(ptrID reflect.Value) {
	// issues detected here are not errors that can be handled,
	// but implementation issues, meaning the function itself is incorrectly written at code level.
	if ptrID.Kind() != reflect.Pointer {
		panic(fmt.Errorf("%w: incorrect extid.ReflectAccessor usage, returned non pointer ID value", reflectkit.ErrTypeMismatch))
	}
	if ptrID.IsNil() {
		panic(fmt.Errorf("%w: incorrect extid.ReflectAccessor usage, function returned a nil ID pointer", reflectkit.ErrTypeMismatch))
	}
}
