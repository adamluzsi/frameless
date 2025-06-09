package extid

import (
	"fmt"
	"reflect"
	"strings"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/zerokit"
)

const errSetWithNonPtr errorkit.Error = "ptr should given as *ENT, else pass by value prevents the ID field remotely"
const errSetWithNonStructENT errorkit.Error = "ENT type was expected to be a struct type"

const ErrIDFieldNotFound errorkit.Error = "ErrIDFieldNotFound"

func Set[ID any](ptr any, id ID) error {
	if ptr == nil {
		return fmt.Errorf("nil given as ptr for extid.Set[%s]", reflectkit.TypeOf[ID]().String())
	}

	var (
		r  = reflect.ValueOf(ptr)
		rt = r.Type()
	)

	if !(rt.Kind() == reflect.Pointer && rt.Elem().Kind() != reflect.Pointer) {
		return errSetWithNonPtr
	}

	if r.IsNil() {
		return fmt.Errorf("nil pointer given for extid.Set[%T]", reflectkit.TypeOf[ID]().String())
	}

	if rt.Elem().Kind() != reflect.Struct {
		return errSetWithNonStructENT
	}

	tr, ok := register[rt.Elem()]
	if ok {
		tr.Set(ptr, id)
		return nil
	}

	lookupByIDType := extractIdentifierFieldByType(idByTypeKey{
		ENT: rt.Elem(),
		ID:  reflectkit.TypeOf[ID](),
	})

	if _, val, ok := lookupByIDType(r.Elem()); ok {
		val.Set(reflect.ValueOf(id))
		return nil
	}

	if _, val, ok := ExtractIdentifierField(ptr); ok {
		val.Set(reflect.ValueOf(id))
		return nil
	}

	return ErrIDFieldNotFound
}

func Get[ID, ENT any](ent ENT) ID {
	id, _ := Lookup[ID, ENT](ent)
	return id
}

// Lookup checks if the given ENT struct type contains a field of type ID.
//
// It returns two values:
// - The ID field, if found.
// - A boolean OK indicating whether an ID field exists in the given ENT struct.
//
// The function prioritises the following when selecting an ID field:
// - Any field of type ID.
// - If multiple fields of type ID exist, the one tagged as 'ext:"id"' is preferred.
//
// This function helps identify the primary ID field in ENT structs consistently.
func Lookup[ID, ENT any](ent ENT) (id ID, ok bool) {
	str := reflectkit.BaseValueOf(ent)
	if tr, ok := register[str.Type()]; ok {
		id := tr.Get(str.Interface()).(ID)
		return id, !zerokit.IsZero(id)
	}
	if _, value, ok := extractIdentifierFieldByType(idByTypeKey{
		ENT: reflectkit.TypeOf[ENT](),
		ID:  reflectkit.TypeOf[ID](),
	})(str); ok {
		id, ok = value.Interface().(ID)
		return id, ok
	}
	if _, value, ok := ExtractIdentifierField(str); ok {
		id, ok = value.Interface().(ID)
		return id, ok
	}
	return id, false
}

type idByTypeKey struct {
	ENT reflect.Type
	ID  reflect.Type
}

var cacheExtractIdentifierFieldByIDType synckit.Map[idByTypeKey, func(reflect.Value) (reflect.StructField, reflect.Value, bool)]

func nullLookup(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
	return reflect.StructField{}, reflect.Value{}, false
}

func extractIdentifierFieldByType(key idByTypeKey) func(reflect.Value) (reflect.StructField, reflect.Value, bool) {
	return cacheExtractIdentifierFieldByIDType.GetOrInit(key, func() func(reflect.Value) (reflect.StructField, reflect.Value, bool) {
		if key.ENT.Kind() != reflect.Struct {
			return nullLookup
		}

		type Hit struct {
			Index int
			Field reflect.StructField
			Tag   reflect.StructTag
		}

		var hits []Hit
		for i := range key.ENT.NumField() {
			field := key.ENT.Field(i)

			if field.Type == key.ID {
				hits = append(hits, Hit{
					Index: i,
					Field: field,
					Tag:   field.Tag,
				})
			}
		}

		if len(hits) == 0 {
			return nullLookup
		}

		if len(hits) == 1 {
			var FieldIndex = hits[0].Index
			return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
				return v.Type().Field(FieldIndex), v.Field(FieldIndex), true
			}
		}

		var (
			candidate Hit
			lastTag   *extTagField
			init      bool
		)
		for _, hit := range hits {
			if !init {
				candidate = hit
			}
			tag, ok, err := extTag.LookupTag(hit.Field)
			if err != nil {
				return nullLookup
			}
			if ok && tag.IsID {
				if lastTag != nil && lastTag.IsID {
					return nullLookup
				}
				candidate = hit
				lastTag = &tag
			}
		}
		if !init {
			return nullLookup
		}

		var FieldIndex = candidate.Index
		return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
			return v.Type().Field(FieldIndex), v.Field(FieldIndex), true
		}
	})
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

type extTagField struct {
	IsID bool
}

var extTag = reflectkit.TagHandler[extTagField]{
	Name: "ext",

	Parse: func(field reflect.StructField, tagName, tagValue string) (extTagField, error) {
		const isIDFlag = "id"
		var tag extTagField
		for _, field := range strings.Fields(tagValue) {
			if strings.EqualFold(field, isIDFlag) {
				tag.IsID = true
			}
		}
		return tag, nil
	},
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
		for i := range val.NumField() {
			tag, ok, err := extTag.LookupTag(val.Type().Field(i))
			if err != nil {
				continue
			}
			if ok && tag.IsID {
				var index = i
				return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
					return v.Type().Field(index), v.Field(index), true
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

func (fn Accessor[ENT, ID]) Get(ent ENT) ID {
	id, _ := fn.Lookup(ent)
	return id
}

func (fn Accessor[ENT, ID]) Lookup(ent ENT) (ID, bool) {
	if fn == nil {
		return Lookup[ID](ent)
	}
	return *fn.ptr(&ent), true
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
