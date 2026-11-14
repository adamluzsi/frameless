package extid

import (
	"fmt"
	"reflect"
	"strings"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
)

const errSetWithNonPtr errorkitlite.Error = "ptr should given as *ENT, else pass by value prevents the ID field remotely"
const errSetWithNonStructENT errorkitlite.Error = "ENT type was expected to be a struct type"

const ErrIDFieldNotFound errorkitlite.Error = "ErrIDFieldNotFound"

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

	// When ENT is an interface type (e.g. workflow.Event), dereference
	// through the interface to reach the underlying concrete struct, mutate
	// a copy of that struct with the new ID, then write it back into the
	// interface value. The interface value obtained via a pointer
	// dereference is addressable, while the struct value held inside the
	// interface is not, hence the copy-and-reassign pattern.
	if rt.Elem().Kind() == reflect.Interface {
		ifaceVal := r.Elem()
		if ifaceVal.IsNil() {
			return fmt.Errorf("nil interface value given for extid.Set[%T]", reflectkit.TypeOf[ID]().String())
		}
		structVal := ifaceVal.Elem()
		if structVal.Kind() != reflect.Struct {
			return errSetWithNonStructENT
		}
		concreteType := structVal.Type()

		tr, ok := register[concreteType]
		if ok {
			newStruct := reflect.New(concreteType).Elem()
			newStruct.Set(structVal)
			tr.Set(newStruct.Addr().Interface(), id)
			ifaceVal.Set(newStruct)
			return nil
		}

		lookupByIDType := extractIdentifierFieldByType(idByTypeKey{
			ENT: concreteType,
			ID:  reflectkit.TypeOf[ID](),
		})

		newStruct := reflect.New(concreteType).Elem()
		newStruct.Set(structVal)
		if sf, val, ok := lookupByIDType(newStruct); ok {
			if err := setIDField(sf, val, id); err != nil {
				return err
			}
			ifaceVal.Set(newStruct)
			return nil
		}

		if sf, val, ok := extractIdentifierField(concreteType, newStruct); ok {
			if err := setIDField(sf, val, id); err != nil {
				return err
			}
			ifaceVal.Set(newStruct)
			return nil
		}

		return ErrIDFieldNotFound
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

	if sf, val, ok := lookupByIDType(r.Elem()); ok {
		return setIDField(sf, val, id)
	}

	if sf, val, ok := ExtractIdentifierField(ptr); ok {
		return setIDField(sf, val, id)
	}

	return ErrIDFieldNotFound
}

// setIDField sets the id to the ID field.
// When the field was found by convention, rather than by the type of the ID,
// the field's type might not be able to hold the id.
func setIDField[ID any](sf reflect.StructField, field reflect.Value, id ID) error {
	rID := reflect.ValueOf(id)
	if !rID.IsValid() { // a nil interface value is only a value of the ID's static type
		rID = reflect.ValueOf(&id).Elem()
	}
	if !rID.Type().AssignableTo(field.Type()) {
		return fmt.Errorf("%w: the %s field's type (%s) can't hold a %s ID",
			ErrIDFieldNotFound, sf.Name, field.Type().String(), rID.Type().String())
	}
	field.Set(rID)
	return nil
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
// - If the struct's own fields are tagged as 'ext:"id"', the tag decides, and untagged fields are not considered.
// - Any field of type ID.
// - If multiple fields of type ID exist, the one tagged as 'ext:"id"' is preferred,
// then the one named ID.
//
// Fields promoted from embedded structs are considered as well, following Go's rules of promotion.
// Unexported fields are not considered.
//
// This function helps identify the primary ID field in ENT structs consistently.
func Lookup[ID, ENT any](ent ENT) (id ID, ok bool) {
	// reflect.ValueOf takes the dynamic value of an interface ENT (e.g. workflow.Event),
	// so its concrete type is used, just like in extid.Set.
	var str = reflect.ValueOf(ent)
	if !str.IsValid() {
		return id, false
	}
	var typ = str.Type()
	// Dereference pointer(s) to reach the underlying value.
	// This mirrors extid.Set, which operates through *ENT,
	// so that Lookup can locate the ID field regardless of whether
	// ENT is a value, a pointer, a pointer-of-pointer,
	// or an interface boxing any of those.
	for typ.Kind() == reflect.Pointer {
		if str.IsNil() {
			var zero ID
			return zero, false
		}
		typ = typ.Elem()
		str = str.Elem()
	}
	if typ.Kind() == reflect.Interface && str.IsNil() { // e.g. a pointer to a nil interface value
		return id, false
	}
	if tr, ok := register[typ]; ok {
		id, ok := tr.Get(str.Interface()).(ID)
		return id, ok
	}
	if _, value, ok := extractIdentifierFieldByType(idByTypeKey{
		ENT: typ,
		ID:  reflectkit.TypeOf[ID](),
	})(str); ok {
		id, ok = value.Interface().(ID)
		return id, ok
	}
	if _, value, ok := extractIdentifierField(typ, str); ok {
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
		field, ok := identifierFieldByType(key.ENT, key.ID)
		if !ok {
			return nullLookup
		}
		return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
			return field, v.FieldByIndex(field.Index), true
		}
	})
}

// identifierFieldByType selects the ID field of the ENT struct type among its fields with the ID type.
//
// When the struct's own fields are tagged as `ext:"id"`, the tag decides the ID field,
// so only the tagged fields are considered.
//
// Fields promoted through an embedded pointer are not considered,
// since that pointer might be nil.
//
// When more than one field has the ID type, the one tagged as `ext:"id"` is selected, else the one named ID.
// If more of them are tagged, the selection is ambiguous, and no field is selected.
func identifierFieldByType(ENT, ID reflect.Type) (reflect.StructField, bool) {
	var hits []reflect.StructField
	for _, field := range idFieldCandidates(ENT) {
		if field.Type != ID || !field.IsExported() || isPromotedThroughPointer(ENT, field.Index) {
			continue
		}
		hits = append(hits, field)
	}
	if len(hits) == 1 {
		return hits[0], true
	}
	var (
		tagged []reflect.StructField
		named  *reflect.StructField
	)
	for i, hit := range hits {
		if isTaggedAsID(hit) {
			tagged = append(tagged, hit)
		}
		if hit.Name == structIDFieldName {
			named = &hits[i]
		}
	}
	if len(tagged) == 1 {
		return tagged[0], true
	}
	if len(tagged) == 0 && named != nil {
		return *named, true
	}
	return reflect.StructField{}, false
}

func isPromotedThroughPointer(typ reflect.Type, index []int) bool {
	for _, i := range index[:len(index)-1] {
		typ = typ.Field(i).Type
		if typ.Kind() == reflect.Pointer {
			return true
		}
	}
	return false
}

// idFieldCandidates returns the fields of the ENT struct type which may hold its ID.
//
// The `ext:"id"` tag is the authority on the ID field,
// so when the struct's own fields are tagged, only the tagged fields are candidates,
// even the unexported ones, which can't be used as the ID, but still express that no other field is the ID.
// Otherwise every visible field is a candidate, including the ones promoted from embedded structs.
func idFieldCandidates(ENT reflect.Type) []reflect.StructField {
	var tagged []reflect.StructField
	for i := range ENT.NumField() {
		if field := ENT.Field(i); isTaggedAsID(field) {
			tagged = append(tagged, field)
		}
	}
	if 0 < len(tagged) {
		return tagged
	}
	return reflect.VisibleFields(ENT)
}

func isTaggedAsID(field reflect.StructField) bool {
	tag, ok, err := extTag.Lookup(field)
	return err == nil && ok && tag.IsID
}

var cacheExtractIdentifierField synckit.Map[reflect.Type, func(reflect.Value) (reflect.StructField, reflect.Value, bool)]

func ExtractIdentifierField(ent any) (reflect.StructField, reflect.Value, bool) {
	val := reflectkit.ToValue(ent)
	val = reflectkit.BaseValue(val)
	if !val.IsValid() {
		return reflect.StructField{}, reflect.Value{}, false
	}
	return extractIdentifierField(val.Type(), val)
}

func extractIdentifierField(typ reflect.Type, val reflect.Value) (reflect.StructField, reflect.Value, bool) {
	// When ENT is an interface type, the cache key must reflect the concrete
	// dynamic type carried by val (e.g. VariableEvent, EventParticipant),
	// not the interface type itself. Otherwise a closure built for one concrete
	// type would be reused for a different one (or for a zero value), which
	// leads to panics such as "reflect: call of reflect.Value.Type on zero Value".
	if typ.Kind() == reflect.Interface {
		if !val.IsValid() || val.Kind() == reflect.Interface && val.IsNil() {
			return reflect.StructField{}, reflect.Value{}, false
		}
		typ = val.Type()
	}
	init := func() func(reflect.Value) (reflect.StructField, reflect.Value, bool) {
		return refMakeExtractFunc(val)
	}
	return cacheExtractIdentifierField.GetOrInit(typ, init)(val)
}

type extTagField struct {
	IsID bool
}

const structIDFieldName = `ID`

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
			field := val.Type().Field(i)
			if !field.IsExported() {
				continue
			}
			tag, ok, err := extTag.Lookup(field)
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
		// The field is looked up on the type, as reflect.Value#FieldByName panics
		// when the field is promoted through a nil embedded pointer.
		if sf, ok := val.Type().FieldByName(structIDFieldName); ok && len(sf.Index) == 1 {
			return func(v reflect.Value) (reflect.StructField, reflect.Value, bool) {
				return sf, v.FieldByIndex(sf.Index), true
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
	return id, true
}

func (fn ReflectAccessor) ReflectSet(ptrENT reflect.Value, id reflect.Value) (rErr error) {
	defer errorkitlite.Recover(&rErr)
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
