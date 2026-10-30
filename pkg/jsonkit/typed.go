package jsonkit

import (
	"bytes"
	"cmp"
	jsonv2 "encoding/json/v2"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit"
)

type WithType[T any] struct{ V T }

var _ jsonv2.Marshaler = WithType[any]{}

func (dto WithType[T]) MarshalJSON() ([]byte, error) {
	return marshalWithType(reflectkit.TypeOf[T](), dto.V)
}

var _ jsonv2.Unmarshaler = (*WithType[any])(nil)

func (dto *WithType[T]) UnmarshalJSON(data []byte) error {
	value, err := unmarshalWithType[T](data)
	if err != nil {
		return err
	}
	dto.V = value
	return nil
}

type WithTypeIDOf[T, IDType any] struct{ V T }

var _ jsonv2.Marshaler = WithTypeIDOf[any, any]{}

func (dto WithTypeIDOf[T, IDType]) MarshalJSON() ([]byte, error) {
	return marshalWithType(reflectkit.TypeOf[T](), dto.V)
}

var _ jsonv2.Unmarshaler = (*WithTypeIDOf[any, any])(nil)

func (dto *WithTypeIDOf[T, IDType]) UnmarshalJSON(data []byte) error {
	value, err := unmarshalWithType[T](data)
	if err != nil {
		return err
	}
	dto.V = value
	return nil
}

// marshalWithType writes v, a value of T, in its type envelope, the way a Codec writes a value held by an interface,
// except that it is written in its envelope, even when plain JSON would be read back as the very same value.
// A value without a type ID is written with the name of T as its type ID, unless its JSON is an object,
// like the one of a struct, which needs a type ID to be read back.
// A null is written as it is, unless T is an interface, as then the envelope tells that it is a nil T.
func marshalWithType(T reflect.Type, v any) ([]byte, error) {
	var (
		c                 Codec
		m                 = newMarshaler(&c)
		rv                = reflect.ValueOf(v)
		typeID, gotTypeID = lookupTypeIDForHeldValue(nil, v)
		data              []byte
		err               error
	)
	switch entries, ok := entriesOfHeldMap(rv, nil); {
	case !rv.IsValid():
		data = []byte("null")
	case ok && (gotTypeID || T.Kind() == reflect.Map):
		// The entries of a map are the map member of the envelope, when it names the type of the map, which it is read back as.
		data, err = m.marshalEntries(entries)
	default:
		data, err = m.marshal(v)
	}
	if err != nil {
		return nil, err
	}
	if T.Kind() != reflect.Interface && isNull(data) {
		return data, nil
	}
	if !gotTypeID && isObject(data) && T.Kind() != reflect.Map && !holdsMap(v) {
		return nil, errMissingTypeID(v, m.kw)
	}
	valueType := T
	if rv.IsValid() {
		valueType = rv.Type()
	}
	if T.Kind() == reflect.Map {
		valueType = T
	}
	return wrapInEnvelope(m.kw, cmp.Or(typeID, TypeID(T.String())), valueType, data)
}

// holdsMap reports whether value is a map, or a pointer to one.
func holdsMap(value any) bool {
	t := reflect.TypeOf(value)
	return t != nil && base(t).Kind() == reflect.Map
}

// unmarshalWithType reads data, which marshalWithType wrote, into a T.
// The type envelope is read the way a Codec reads a value into an interface, see unmarshaler.readTyped,
// while JSON without a type envelope is read straight into T, as T already tells its type.
// null is the zero value of T, even for a T with its own UnmarshalJSON, as that is what a nil T is written as.
func unmarshalWithType[T any](data []byte) (T, error) {
	var (
		c   Codec
		u   = newUnmarshaler(&c)
		typ = reflectkit.TypeOf[T]()
		ptr = reflect.New(typ)
	)
	data = bytes.TrimSpace(data)
	env, err := readEnvelope(data, u.kw)
	switch {
	case err != nil:
		return *new(T), err
	case env.Found:
		value, err := u.readObject(data, typ)
		if err != nil {
			return *new(T), err
		}
		return typeAssert[T](value.Interface())
	case typ.Kind() == reflect.Interface && isObject(data):
		// Unlike the JSON value of an `any` field, which may be a plain JSON object, it is asked to have a type envelope.
		return *new(T), fmt.Errorf("%s is not set", u.kw.Type)
	case typ.Kind() != reflect.Interface && isNull(data):
		return *new(T), nil
	}
	if err := u.unmarshal(data, ptr.Interface()); err != nil {
		return *new(T), err
	}
	return *ptr.Interface().(*T), nil
}
