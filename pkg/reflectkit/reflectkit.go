package reflectkit

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"strings"
	"unsafe"

	"go.llib.dev/frameless/internal/interr"
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

type TagHandler[T any] struct {
	Name  string
	Parse func(sf reflect.StructField, tagValue string) (T, error)
	Use   func(sf reflect.StructField, field reflect.Value, v T) error
	cache synckit.Map[tagHandlerCacheKey, T]
	// ForceCache will force the TagHandler to cache the parse results, regardless if the value is mutable or not.
	ForceCache bool
	// HandleUntagged will force the Handle functions to call Parse and Use even on fields where tag is empty.
	HandleUntagged bool
}

func (h *TagHandler[T]) HandleStruct(rStruct reflect.Value) error {
	if !rStruct.IsValid() {
		return interr.ImplementationError.F("valid struct value was expected")
	}
	if rStruct.Kind() != reflect.Struct {
		return interr.ImplementationError.F("%s is not a struct type", rStruct.Type().String())
	}
	for sf, val := range OverStruct(rStruct) {
		if err := h.handleStructField(sf, val); err != nil {
			return err
		}
	}
	return nil
}

func (h *TagHandler[T]) HandleStructField(field reflect.StructField, value reflect.Value) error {
	if !h.isStructFieldOK(field) {
		return interr.ImplementationError.F("invalid struct field type description received")
	}
	if !value.IsValid() {
		return interr.ImplementationError.F("invalid struct field value received")
	}
	if h.Parse == nil {
		return interr.ImplementationError.F("missing %T.Parse", h)
	}
	if h.Use == nil {
		return interr.ImplementationError.F("missing %T.Use", h)
	}
	return h.handleStructField(field, value)
}

func (h *TagHandler[T]) handleStructField(field reflect.StructField, value reflect.Value) error {
	v, ok, err := h.LookupTag(field)
	if err != nil {
		return err
	}
	if !ok && !h.HandleUntagged {
		return nil
	}
	if err := h.Use(field, value, v); err != nil {
		return err
	}
	return nil
}

func (h *TagHandler[T]) LookupTag(field reflect.StructField) (T, bool, error) {
	tag, ok := field.Tag.Lookup(h.Name)
	if !ok && !h.HandleUntagged {
		var zero T
		return zero, ok, nil
	}
	v, err := h.parse(field, tag)
	if err != nil {
		var zero T
		return zero, ok, fmt.Errorf("%T.Parse failed: %w", h, err)
	}
	return v, true, nil
}

func (h *TagHandler[T]) parse(sf reflect.StructField, tagValue string) (T, error) {
	var tagValueType = TypeOf[T]()
	if !h.ForceCache && IsMutableType(tagValueType) {
		return h.Parse(sf, tagValue)
	}
	key := tagHandlerCacheKey{
		TagValueType:    FullyQualifiedName(tagValueType),
		StructFieldName: sf.Name,
		StructFieldType: FullyQualifiedName(sf.Type),
		StructFieldTag:  sf.Tag,
	}
	return h.cache.GetOrInitErr(key, func() (T, error) {
		// we only need to parse once the tag, not more
		// since the tag itself is a constant value
		// that only change when the source code is changed.
		return h.Parse(sf, tagValue)
	})
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

// Clone recursively creates a deep copy of a reflect.Value
func Clone(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Value{}
	}
	switch value.Kind() {
	case reflect.Ptr:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copy := reflect.New(value.Type().Elem())
		copy.Elem().Set(Clone(value.Elem()))
		return copy

	case reflect.Struct:
		copy := reflect.New(value.Type()).Elem()
		num := value.NumField()
		for i := 0; i < num; i++ {
			dst := copy.Field(i)
			var ok bool
			dst, ok = ToSettable(dst)
			if !ok {
				continue
			}
			src := value.Field(i)
			dst.Set(Clone(src))
		}
		return copy

	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copy := reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
		for i := 0; i < value.Len(); i++ {
			copy.Index(i).Set(Clone(value.Index(i)))
		}
		return copy

	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		copy := reflect.MakeMapWithSize(value.Type(), value.Len())
		for _, key := range value.MapKeys() {
			copy.SetMapIndex(key, Clone(value.MapIndex(key)))
		}
		return copy

	case reflect.Chan:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		return reflect.MakeChan(value.Type(), value.Cap())

	default:
		return reflect.ValueOf(value.Interface())

	}
}

func OverStruct(rStruct reflect.Value) iter.Seq2[reflect.StructField, reflect.Value] {
	if rStruct.Kind() != reflect.Struct {
		panic(interr.ImplementationError.F("expected %s to be a struct type", rStruct.Type().String()))
	}
	return iter.Seq2[reflect.StructField, reflect.Value](func(yield func(reflect.StructField, reflect.Value) bool) {
		var (
			typ = rStruct.Type()
			num = typ.NumField()
		)
		for i := 0; i < num; i++ {
			if !yield(typ.Field(i), rStruct.Field(i)) {
				break
			}
		}
	})
}

func OverMap(rMap reflect.Value) iter.Seq2[reflect.Value, reflect.Value] {
	if rMap.Kind() != reflect.Map {
		panic(interr.ImplementationError.F("expected %s to be a map type", rMap.Type().String()))
	}
	return iter.Seq2[reflect.Value, reflect.Value](func(yield func(reflect.Value, reflect.Value) bool) {
		i := rMap.MapRange()
		for i.Next() {
			if !yield(i.Key(), i.Value()) {
				break
			}
		}
	})
}

func OverSlice(rSlice reflect.Value) iter.Seq2[int, reflect.Value] {
	if rSlice.Kind() != reflect.Slice {
		panic(interr.ImplementationError.F("expected %s to be a slice type", rSlice.Type().String()))
	}
	return iter.Seq2[int, reflect.Value](func(yield func(int, reflect.Value) bool) {
		var length = rSlice.Len()
		for i := 0; i < length; i++ {
			if !yield(i, rSlice.Index(i)) {
				break
			}
		}
	})
}

func IsBuiltInType(typ reflect.Type) bool {
	return typ.PkgPath() == ""
}
