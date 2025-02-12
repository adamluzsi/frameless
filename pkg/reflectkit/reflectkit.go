package reflectkit

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/synckit"
)

const ErrTypeMismatch errorkit.Error = "ErrTypeMismatch"

const ErrInvalid errorkit.Error = "ErrInvalid"

func Cast[T any](v any) (T, bool) {
	var (
		typ = reflect.TypeOf(*new(T))
		val = reflect.ValueOf(v)
	)
	if !val.CanConvert(typ) {
		return *new(T), false
	}
	return val.Convert(typ).Interface().(T), true
}

func DerefType(T reflect.Type) (_ reflect.Type, depth int) {
	if T == nil {
		return T, depth
	}
	for ; T.Kind() == reflect.Pointer; depth++ {
		T = T.Elem()
	}
	return T, depth
}

func PointerOf(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	if value.CanAddr() {
		return value.Addr()
	}
	ptr := reflect.New(value.Type())
	ptr.Elem().Set(value)
	return ptr
}

func BaseTypeOf(i any) reflect.Type {
	t := reflect.TypeOf(i)

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}

func BaseValueOf(i any) reflect.Value {
	return BaseValue(reflect.ValueOf(i))
}

func isBaseKind(kind reflect.Kind) bool {
	return kind != reflect.Pointer && kind != reflect.Interface
}

func BaseValue(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	for !isBaseKind(v.Kind()) {
		v = v.Elem()
	}
	return v
}

func baseTypeOfAny(v any) (reflect.Type, int) {
	var typ reflect.Type
	switch v := v.(type) {
	case reflect.Type:
		typ = v
	case reflect.Value:
		typ = v.Type()
	default:
		typ = reflect.TypeOf(v)
	}
	return DerefType(typ)
}

func SymbolicName(v any) string {
	typ, depth := baseTypeOfAny(v)
	return strings.Repeat("*", depth) + typ.String()
}

func FullyQualifiedName(v any) string {
	typ, depth := baseTypeOfAny(v)
	var name = typ.Name()
	if pkgPath := typ.PkgPath(); pkgPath != "" {
		name = fmt.Sprintf("%q.%s", pkgPath, name)
	}
	if depth == 0 {
		return name
	}
	return strings.Repeat("*", depth) + name
}

func IsEmpty(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Interface:
		return IsEmpty(val.Elem())
	case reflect.Slice, reflect.Map:
		if val.IsNil() {
			return true
		}
		return val.Len() == 0
	case reflect.Pointer:
		if val.IsNil() {
			return true
		}
		return IsEmpty(val.Elem())
	case reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return !val.IsValid() || val.IsZero()
	}
}

func IsZero(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return !val.IsValid() || val.IsZero()
	}
}

func IsNil(val reflect.Value) bool {
	switch val.Kind() {
	case reflect.Slice, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Chan, reflect.Func:
		return val.IsNil()
	default:
		return false
	}
}

// Link will make destination interface be linked with the src value.
func Link(src, ptr any) (err error) {
	vPtr := reflect.ValueOf(ptr)

	if vPtr.Kind() != reflect.Pointer {
		return errors.New(`pointer type destination expected`)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New(fmt.Sprint(recovered))
		}
	}()

	vPtr.Elem().Set(reflect.ValueOf(src))

	return nil
}

// SetValue will force set
func SetValue(variable, value reflect.Value) {
	if variable.CanSet() {
		variable.Set(value)
		return
	}
	reflect.NewAt(variable.Type(), unsafe.Pointer(variable.UnsafeAddr())).
		Elem().Set(value)
}

var anyInterface = reflect.TypeOf((*any)(nil)).Elem()

func TypeOf[T any](i ...T) reflect.Type {
	var typ = reflect.TypeOf((*T)(nil)).Elem()
	if 0 < len(i) && typ == anyInterface {
		for _, v := range i {
			if typeOfV := reflect.TypeOf(v); typeOfV != nil {
				return typeOfV
			}
		}
	}
	return typ
}

func ToValue(v any) reflect.Value {
	if rv, ok := v.(reflect.Value); ok {
		return rv
	}
	return reflect.ValueOf(v)
}

func LookupField[FieldID LookupFieldID](rStruct reflect.Value, i FieldID) (reflect.StructField, reflect.Value, bool) {
	if rStruct.Kind() != reflect.Struct || !rStruct.IsValid() {
		return reflect.StructField{}, reflect.Value{}, false
	}

	structField, ok := toStructField[FieldID](rStruct.Type(), i)
	if !ok {
		return reflect.StructField{}, reflect.Value{}, false
	}

	field := rStruct.FieldByIndex(structField.Index)
	if !field.IsValid() {
		return structField, field, false
	}

	if af, ok := toAccessible(field); ok {
		field = af
	}

	return structField, field, field.IsValid()
}

func toStructField[FieldID LookupFieldID](rStructType reflect.Type, i FieldID) (reflect.StructField, bool) {
	switch i := any(i).(type) {
	case reflect.StructField:
		return i, isStructFieldOK(i)
	case int:
		sf := rStructType.Field(i)
		return sf, isStructFieldOK(sf)
	case string:
		return rStructType.FieldByName(i)
	default:
		panic("unknown reflectkit.LookupFieldID type")
	}
}

func isStructFieldOK(sf reflect.StructField) bool {
	return sf.Name != "" && 0 < len(sf.Index)
}

type LookupFieldID interface {
	/* StructField */ reflect.StructField | /*index*/ int | /* name */ string
}

func ToSettable(rv reflect.Value) (_ reflect.Value, ok bool) {
	if !rv.IsValid() {
		return reflect.Value{}, false
	}
	if rv.CanSet() {
		return rv, true
	}
	if rv.CanAddr() {
		if uv := reflect.NewAt(rv.Type(), rv.Addr().UnsafePointer()).Elem(); uv.CanInterface() {
			return uv, true
		}
	}
	return reflect.Value{}, false
}

type StructFieldID struct {
	Path string
	Name string
	Type string
	Tag  string
}

func ToStructFieldID[FieldID LookupFieldID](rStructType reflect.Type, id FieldID) StructFieldID {
	sf, ok := toStructField[FieldID](rStructType, id)
	if !ok {
		panic("implementation error, struct value and field id is not related")
	}
	var fieldType string
	if sf.Type != nil {
		fieldType = sf.Type.String()
	}
	return StructFieldID{
		Path: FullyQualifiedName(rStructType),
		Name: sf.Name,
		Type: fieldType,
		Tag:  string(sf.Tag),
	}
}

type TagHandler[T any] struct {
	Name  string
	Parse func(sf reflect.StructField, tag string) (T, error)
	Use   func(sf reflect.StructField, field reflect.Value, v T) error

	cache synckit.Map[tagHandlerCacheKey, T]
}

func (h *TagHandler[T]) Apply(rStuct reflect.Value) error {
	if !rStuct.IsValid() {
		return errorkit.ImplementationError.F("valid struct value was expected")
	}
	if rStuct.Kind() != reflect.Struct {
		return errorkit.ImplementationError.F("%s is not a struct type", rStuct.Type().String())
	}
	if h.Parse == nil {
		return errorkit.ImplementationError.F("missing %T.Parse", h)
	}
	if h.Use == nil {
		return errorkit.ImplementationError.F("missing %T.Use", h)
	}

	var (
		rStuctType = rStuct.Type()
		NumField   = rStuctType.NumField()
	)
	for i := 0; i < NumField; i++ {
		sf := rStuctType.Field(i)

		tag, ok := sf.Tag.Lookup(h.Name)
		if !ok {
			continue
		}

		v, err := h.parse(sf, tag)
		if err != nil {
			return fmt.Errorf("%T.Parse failed: %w", h, err)
		}

		if err := h.Use(sf, rStuct.Field(i), v); err != nil {
			return err
		}
	}

	return nil
}

func (h *TagHandler[T]) ApplyToStructField(sf reflect.StructField, field reflect.Value) error {
	if !h.isStructFieldOK(sf) {
		return errorkit.ImplementationError.F("invalid struct field type description received")
	}
	if !field.IsValid() {
		return errorkit.ImplementationError.F("invalid struct field value received")
	}
	if h.Parse == nil {
		return errorkit.ImplementationError.F("missing %T.Parse", h)
	}
	if h.Use == nil {
		return errorkit.ImplementationError.F("missing %T.Use", h)
	}

	tag, ok := sf.Tag.Lookup(h.Name)
	if !ok {
		return nil
	}

	v, err := h.parse(sf, tag)
	if err != nil {
		return fmt.Errorf("%T.Parse failed: %w", h, err)
	}

	if err := h.Use(sf, field, v); err != nil {
		return err
	}

	return nil
}

func (h *TagHandler[T]) parse(sf reflect.StructField, tag string) (T, error) {
	var tagValueType = TypeOf[T]()
	if IsMutableType(tagValueType) {
		return h.Parse(sf, tag)
	}
	return h.cache.GetOrInitErr(h.cacheKey(sf, tagValueType), func() (T, error) {
		// we only need to parse once the tag, not more
		// since the tag itself is a constant value
		// that only change when the source code is changed.
		return h.Parse(sf, tag)
	})
}

func (h *TagHandler[T]) cacheKey(sf reflect.StructField, tagValueType reflect.Type) tagHandlerCacheKey {
	return tagHandlerCacheKey{
		TagValueType:    FullyQualifiedName(tagValueType),
		StructFieldName: sf.Name,
		StructFieldType: FullyQualifiedName(sf.Type),
		StructFieldTag:  sf.Tag,
	}
}

type tagHandlerCacheKey struct {
	TagValueType    string
	StructFieldName string
	StructFieldType string
	StructFieldTag  reflect.StructTag
}

func (h *TagHandler[T]) isStructFieldOK(sf reflect.StructField) bool {
	return sf.Type != nil && sf.Index != nil && sf.Name != ""
}
