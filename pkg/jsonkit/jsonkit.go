package jsonkit

import (
	"bytes"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
)

type Array[T any] []T

// MarshalJSON writes the Array as a JSON array.
// When T is an interface, the elements are written by a Codec,
// so a value held by an interface is written the same way as with the Codec, with the package level registry.
// Like with encoding/json, a nil Array is written as null.
func (ary Array[T]) MarshalJSON() ([]byte, error) {
	if !isInterfaceType(reflectkit.TypeOf[T]()) {
		return jsonv2.Marshal([]T(ary), v1Options)
	}
	return Codec{}.Marshal(ary)
}

// UnmarshalJSON reads a JSON array into the Array.
// When T is an interface, the elements are read by a Codec, the same way as with the Codec, with the package level registry.
// Like with encoding/json, null is read as a nil Array.
func (ary *Array[T]) UnmarshalJSON(data []byte) error {
	if !isInterfaceType(reflectkit.TypeOf[T]()) {
		v := []T(*zerokit.Coalesce(ary, &Array[T]{}))
		if err := jsonv2.Unmarshal(data, &v, v1Options); err != nil {
			return err
		}
		*ary = v
		return nil
	}
	// Read into a new Array, so a failed read keeps the Array as it was.
	var vs Array[T]
	if err := (Codec{}).Unmarshal(data, &vs); err != nil {
		return err
	}
	*ary = vs
	return nil
}

func isInterfaceType(T reflect.Type) bool {
	return T.Kind() == reflect.Interface
}

type TypeID string

func (id TypeID) String() string { return string(id) }

func (id TypeID) IsZero() bool {
	const zero TypeID = ""
	return id == zero
}

// maxPtrNesting caps how many pointer levels newImpl wraps around the
// identified type while it looks for one that satisfies the target type.
const maxPtrNesting = 42

func newImpl(targetType, identifiedType reflect.Type) (reflect.Value, error) {
	var check func(reflect.Type) bool
	switch targetType.Kind() {
	case reflect.Interface:
		check = func(typ reflect.Type) bool {
			return typ.Implements(targetType)
		}
	default:
		check = func(typ reflect.Type) bool {
			return typ.ConvertibleTo(targetType)
		}
	}

	ptr := reflect.New(identifiedType)
	for range maxPtrNesting {
		if check(ptr.Type().Elem()) {
			return ptr, nil
		}
		ptrPtr := reflect.New(ptr.Type())
		ptrPtr.Elem().Set(ptr)
		ptr = ptrPtr
	}

	return reflect.Value{}, errNoImplementation(targetType, identifiedType)
}

func errNoImplementation(targetType, identifiedType reflect.Type) error {
	return fmt.Errorf("unable to find implementation for %s with %s", targetType.String(), identifiedType.String())
}

// setDecoded stores a decoded value into dst.
// An invalid value, such as a decoded JSON null, sets dst to its zero value.
//
// A value that dst can't hold is reported as an error instead of a reflect.Value#Set panic.
// For example, an untyped JSON array decodes into []any, which a non-empty interface can't hold.
// The same goes for a registered $type that doesn't implement the destination interface.
func setDecoded(dst, value reflect.Value) error {
	if !value.IsValid() {
		dst.SetZero()
		return nil
	}
	if !value.Type().AssignableTo(dst.Type()) {
		return errNoImplementation(dst.Type(), value.Type())
	}
	dst.Set(value)
	return nil
}

func isObject(data []byte) bool {
	// A collection of key/value pairs where the keys are strings,
	// and the values can be any JSON type.
	// Objects in JSON are similar to dictionaries in Python, hashes in Ruby, or maps in Go and Java.
	// They are enclosed in curly braces ({}).
	return bytes.HasPrefix(data, curlyBracketOpen) &&
		bytes.HasSuffix(data, curlyBracketClose)
}

func isNull(data []byte) bool {
	// Represents a null value, indicating the absence of any value.
	return bytes.Equal(data, null)
}

const ErrNotInterfaceType errorkit.Error = "jsonkit.ErrNotInterfaceType"

// ErrUnknownTypeID is the error of decoding data whose $type names no type of a value that the Codec knows.
// Either no type is registered under the type ID, or the type ID names an interface type, which no value is of.
const ErrUnknownTypeID errorkit.Error = "jsonkit: unknown $type"

// ErrKeywordCollision is the error of writing a value whose own member is named like a keyword of the Codec,
// where it would be read back as a member of the type envelope, instead of as a member of the value,
// like a member named "$type" of a struct held by an `any`, or a "$type" key of a map written as plain JSON, without an envelope.
// The Codec#TypeKeyword, Codec#ValueKeyword and Codec#MapKeyword name the members of the type envelope otherwise.
// It is also the error of a Codec where two of these keywords are the same.
const ErrKeywordCollision errorkit.Error = "jsonkit: keyword collision"

type Interface[I any] struct{ V I }

// MarshalJSON writes the value the interface holds with its $type, the same way as with a Codec, with the package level registry.
func (i Interface[I]) MarshalJSON() ([]byte, error) {
	if !isInterfaceType(reflectkit.TypeOf[I]()) {
		return nil, ErrNotInterfaceType
	}
	return Codec{}.Marshal(i)
}

// UnmarshalJSON reads the value by its $type, the same way as with a Codec, with the package level registry.
func (i *Interface[I]) UnmarshalJSON(data []byte) error {
	if !isInterfaceType(reflectkit.TypeOf[I]()) {
		return ErrNotInterfaceType
	}
	// Read into a new Interface, so a failed read keeps the Interface as it was.
	var got Interface[I]
	if err := (Codec{}).Unmarshal(data, &got); err != nil {
		return err
	}
	*i = got
	return nil
}

// typeAssert asserts that a decoded value is a T, following the same rules as setDecoded.
// A nil value, such as a decoded JSON null, is the zero value of T.
func typeAssert[T any](v any) (T, error) {
	var value T
	if err := setDecoded(reflect.ValueOf(&value).Elem(), reflect.ValueOf(v)); err != nil {
		return *new(T), err
	}
	return value, nil
}

func Indent(src []byte, prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	err := json.Indent(&buf, src, prefix, indent)
	return buf.Bytes(), err
}
