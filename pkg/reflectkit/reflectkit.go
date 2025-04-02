package reflectkit

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"strings"
	"time"
	"unsafe"

	"go.llib.dev/frameless/internal/interr"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit/internal"
	"go.llib.dev/frameless/pkg/synckit"
)

const ErrTypeMismatch = internal.ErrTypeMismatch

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

func baseType(v any) (reflect.Type, int) {
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
	typ, depth := baseType(v)
	return strings.Repeat("*", depth) + typ.String()
}

func FullyQualifiedName(v any) string {
	typ, depth := baseType(v)
	var name = typ.Name()
	if len(name) == 0 {
		name = typ.String()
	}
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

var nilables = map[reflect.Kind]struct{}{
	reflect.Slice:     {},
	reflect.Map:       {},
	reflect.Pointer:   {},
	reflect.Interface: {},
	reflect.Chan:      {},
	reflect.Func:      {},
}

func IsNilable[T reflect.Kind | reflect.Value](v T) bool {
	switch v := any(v).(type) {
	case reflect.Kind:
		_, ok := nilables[v]
		return ok
	case reflect.Value:
		_, ok := nilables[v.Kind()]
		return ok
	default:
		panic("not-implemented")
	}
}

func IsNil(val reflect.Value) bool {
	if !IsNilable(val) {
		return false
	}
	return val.IsNil()
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

func ToType(T any) reflect.Type {
	switch T := T.(type) {
	case reflect.Type:
		return T
	case reflect.Value:
		return T.Type()
	default:
		return reflect.TypeOf(T)
	}
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
		field := rStructType.Field(i)
		return field, isStructFieldOK(field)
	case string:
		return rStructType.FieldByName(i)
	default:
		panic("unknown reflectkit.LookupFieldID type")
	}
}

func isStructFieldOK(field reflect.StructField) bool {
	return field.Name != "" && 0 < len(field.Index)
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
	Name string
	// Alias is a list of optional tag alias that will be checked if Name is not avaialble.
	Alias []string
	// Parse meant to interpret the content of a raw tag and converts it into a tag value.
	//
	// Ideally, parsing occurs only once, provided the tag value remains immutable.
	// If ForceCache is enabled, a successful Parse is guaranteed to run only once.
	//
	// If an unrecoverable error occurs during parsing, such as invalid tag format,
	// consider either panicking or enabling PanicOnParseError to handle the failure.
	Parse func(field reflect.StructField, tagValue string) (T, error)
	// Use specifies what should be done with the parse tag value
	// while the tag is being applied on a given struct field.
	Use func(field reflect.StructField, value reflect.Value, v T) error
	// ForceCache will force the TagHandler to cache the parse results, regardless if the value is mutable or not.
	ForceCache bool
	// HandleUntagged will force the Handle functions to call Parse and Use even on fields where tag is empty.
	HandleUntagged bool
	// PanicOnParseError triggers a panic if a parsing error occurs.
	//
	// Enabling this flag enforces strict tag formatting,
	// as tags are determined at compile time, making runtime fixes impossible.
	PanicOnParseError bool

	cache synckit.Map[tagHandlerCacheKey, T]
}

func (h *TagHandler[T]) HandleStruct(rStruct reflect.Value) error {
	if !rStruct.IsValid() {
		return interr.ImplementationError.F("valid struct value was expected")
	}
	if rStruct.Kind() != reflect.Struct {
		return interr.ImplementationError.F("%s is not a struct type", rStruct.Type().String())
	}
	for field, value := range OverStruct(rStruct) {
		if err := h.handleStructField(field, value); err != nil {
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
	if !ok && 0 < len(h.Alias) {
		for _, alias := range h.Alias {
			tag, ok = field.Tag.Lookup(alias)
			if ok {
				break
			}
		}
	}
	if !ok && !h.HandleUntagged {
		var zero T
		return zero, ok, nil
	}
	v, err := h.parse(field, tag)
	if err != nil {
		if h.PanicOnParseError {
			panic(err)
		}
		var zero T
		return zero, ok, err
	}
	return v, true, nil
}

func (h *TagHandler[T]) parse(field reflect.StructField, tagValue string) (T, error) {
	var tagValueType = TypeOf[T]()
	if !h.ForceCache && IsMutableType(tagValueType) {
		return h.Parse(field, tagValue)
	}
	key := tagHandlerCacheKey{
		TagValueType:    FullyQualifiedName(tagValueType),
		StructFieldName: field.Name,
		StructFieldType: FullyQualifiedName(field.Type),
		StructFieldTag:  field.Tag,
	}
	return h.cache.GetOrInitErr(key, func() (T, error) {
		// we only need to parse once the tag, not more
		// since the tag itself is a constant value
		// that only change when the source code is changed.
		return h.Parse(field, tagValue)
	})
}

type tagHandlerCacheKey struct {
	TagValueType    string
	StructFieldName string
	StructFieldType string
	StructFieldTag  reflect.StructTag
}

func (h *TagHandler[T]) isStructFieldOK(field reflect.StructField) bool {
	return field.Type != nil && field.Index != nil && field.Name != ""
}

// Clone recursively creates a deep copy of a reflect.Value
func Clone(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Value{}
	}
	switch value.Kind() {
	case reflect.Pointer:
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

// Comparable is an optional interface type that allows implementing types to perform value comparisons.
//
// Types implementing this interface must provide a Compare method that defines the ordering or equivalence of values.
// This pattern is useful when working with:
// 1. Custom user-defined types requiring comparison logic
// 2. Encapsulated values needing semantic comparisons
// 3. Comparison-agnostic systems (e.g., sorting algorithms)
//
// Example usage:
//
//	type MyNumber struct {
//	    Value int
//	}
//
//	func (m MyNumber) Compare(other MyNumber) int {
//	    if m.Value < other.Value { return -1 }
//	    if m.Value > other.Value { return +1 }
//	    return 0
//	}
type Comparable[T any] interface {
	// Compare returns:
	//   -1 if receiver is less than the argument,
	//    0 if they're equal, and
	//    +1 if receiver is greater.
	//
	// Implementors must ensure consistent ordering semantics.
	Compare(T) int
}

type CmpComparable[T any] interface {
	Cmp(T) int
}

const ErrNotComparable errorkit.Error = "ErrNotComparable"

// Compare will compare "a" and "b" and return the comparison result.
//
//   - -1 if "a" is smaller than "b"
//   - 0  if "a" is equal   to   "b"
//   - 1  if "a" is bigger  than "b"
func Compare[T reflect.Value | any](a, b T) (int, error) {
	if cmp, ok := tryTypedCompare[T](a, b); ok {
		return cmp, nil
	}
	return reflectCompare(ToValue(a), ToValue(b))
}

func tryTypedCompare[T any](a, b T) (int, bool) {
	if _, ok := any(a).(reflect.Value); ok {
		return 0, false
	}
	switch a := any(a).(type) {
	case Comparable[T]:
		return a.Compare(b), true
	case CmpComparable[T]:
		return a.Cmp(b), true
	case float32:
		return compareNumbares(a, any(b).(float32)), true
	case float64:
		return compareNumbares(a, any(b).(float64)), true
	case int:
		return compareNumbares(a, any(b).(int)), true
	case int8:
		return compareNumbares(a, any(b).(int8)), true
	case int16:
		return compareNumbares(a, any(b).(int16)), true
	case int32:
		return compareNumbares(a, any(b).(int32)), true
	case int64:
		return compareNumbares(a, any(b).(int64)), true
	case uint:
		return compareNumbares(a, any(b).(uint)), true
	case uint8:
		return compareNumbares(a, any(b).(uint8)), true
	case uint16:
		return compareNumbares(a, any(b).(uint16)), true
	case uint32:
		return compareNumbares(a, any(b).(uint32)), true
	case uint64:
		return compareNumbares(a, any(b).(uint64)), true
	case string:
		return strings.Compare(a, any(b).(string)), true
	default:
		return 0, false
	}
}

func reflectCompare(a, b reflect.Value) (int, error) {
	if compare, ok := internal.ImplementsComparable(a.Type()); ok {
		return compare(a, b)
	}
	for canElem(a) && canElem(b) {
		a, b = a.Elem(), b.Elem()

		if compare, ok := internal.ImplementsComparable(a.Type()); ok {
			return compare(a, b)
		}
	}
	if a.Type() != b.Type() {
		return 0, ErrTypeMismatch.F("comparison between %s and %s is not possible.", a.Type().String(), b.Type().String())
	}
	if a.CanInt() {
		return compareNumbares(a.Int(), b.Int()), nil
	}
	if a.CanUint() {
		return compareNumbares(a.Uint(), b.Uint()), nil
	}
	if a.CanFloat() {
		return compareNumbares(a.Float(), b.Float()), nil
	}
	if a.Kind() == reflect.String {
		return strings.Compare(a.String(), b.String()), nil
	}
	return 0, ErrNotComparable.F("%s <=/=> %s", a.Type().String(), b.Type().String())
}

type number interface {
	float32 | float64 |
		int | int8 | int16 | int32 | int64 |
		uint | uint8 | uint16 | uint32 | uint64
}

func compareNumbares[T number](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareTime(a, b time.Time) int {
	if a.Before(b) {
		return -1
	} else if a.After(b) {
		return 1
	}
	return 0
}

func canElem(val reflect.Value) bool {
	kind := val.Kind()
	can := kind == reflect.Pointer || kind == reflect.Interface
	if !can {
		return false
	}
	return !val.IsNil()
}
