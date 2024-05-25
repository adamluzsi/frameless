package jsondto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
)

type typed struct {
	Type  reflect.Type
	Value any
	Force bool
}

type typedContainer struct {
	Type  TypeID          `json:"__type"`
	Value json.RawMessage `json:"__value"`
}

// MarshalJSON will marshal Typed T value with a __type property
//
//goland:noinspection GoMixedReceiverTypes
func (v typed) MarshalJSON() ([]byte, error) {
	const __type = `__type`

	data, err := json.Marshal(v.Value)
	if err != nil {
		return nil, err
	}

	isInterfaceType := v.Type.Kind() == reflect.Interface

	if !isInterfaceType {
		switch {
		case isNull(data), isPrimitive(data) && !v.Force:
			return data, nil
		}
	}

	var (
		typeID    TypeID
		gotTypeID bool
	)
	if typeID.IsZero() {
		typeID, gotTypeID = typeIDRegistry.TypeIDFor(v.Value)
	}

	switch {
	case isObject(data) && (isInterfaceType || v.Force):
		data = bytes.TrimPrefix(data, curlyBracketOpen)
		if !bytes.HasPrefix(data, curlyBracketClose) {
			data = append(append([]byte{}, fieldSep...), data...)
		}
		if !gotTypeID {
			return nil, fmt.Errorf("missing __type id alias for %T", v.Value)
		}
		typeIDPart, err := json.Marshal(map[string]TypeID{__type: typeID})
		if err != nil {
			return nil, err
		}
		typeIDPart = bytes.TrimSuffix(typeIDPart, curlyBracketClose)
		data = append(append([]byte{}, typeIDPart...), data...)

	case v.Force:
		data, err = json.Marshal(typedContainer{
			Type:  zerokit.Coalesce(typeID, TypeID(v.Type.String())),
			Value: data,
		})
		if err != nil {
			return nil, err
		}
	}

	if !json.Valid(data) {
		return nil, fmt.Errorf("json marshaling failed for %T.\n%s",
			v.Value, string(data))
	}

	return data, nil
}

// UnmarshalJSON will unserialize the T from data.
//
//goland:noinspection GoMixedReceiverTypes
func (v *typed) UnmarshalJSON(data []byte) error {
	isInterfaceType := v.Type.Kind() == reflect.Interface

	if !isInterfaceType {
		switch {
		case isNull(data), isPrimitive(data) && !v.Force:
			var val typed
			if err := json.Unmarshal(data, &val.Value); err != nil {
				return err
			}
			*v = val
			return nil
		}
	}

	if isObject(data) {
		return v.unmarshalObject(data)
	}

	var val typed
	if err := json.Unmarshal(data, &val.Value); err != nil {
		return err
	}
	*v = val
	return nil
}

//goland:noinspection GoMixedReceiverTypes
func (v *typed) unmarshalObject(data []byte) error {
	var typeID TypeID

	if v.Force {
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		var tc typedContainer
		err := d.Decode(&tc)
		if err == nil {
			data = tc.Value
			typeID = tc.Type
		}
	}

	if typeID.IsZero() {
		type Probe struct {
			TypeID *TypeID `json:"__type,omitempty"`
		}

		var p Probe
		if err := json.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("unable to unmarshal __type field for:\n%s", string(data))
		}
		if p.TypeID == nil {
			return fmt.Errorf("__type is not set")
		}
		if *p.TypeID == "" {
			return fmt.Errorf("__type is empty")
		}
		typeID = *p.TypeID
	}

	var value reflect.Value
	rType, ok := typeIDRegistry.TypeByID(typeID)
	if !ok { // try for primitives
		var val any
		if err := json.Unmarshal(data, &val); err != nil {
			return err
		}
		if TypeID(v.Type.String()) != typeID {
			return fmt.Errorf("%s is not a recognised primitive type", typeID)
		}
		if val == nil {
			value = reflect.Zero(v.Type)
		} else {
			value = reflect.ValueOf(val)
		}
	} else {
		ptr, err := newImpl(v.Type, rType)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, ptr.Interface()); err != nil {
			return err
		}
		value = ptr.Elem()
	}

	vT := reflect.New(v.Type)
	vT.Elem().Set(value)
	v.Value = vT.Elem().Interface()
	return nil
}

type Array[T any] []T

func (ary Array[T]) MarshalJSON() ([]byte, error) {
	if !isInterfaceType[T]() {
		return json.Marshal([]T(ary))
	}

	var vs = make([]json.RawMessage, len(ary))
	for i, v := range ary {
		data, err := json.Marshal(typed{Type: reflectkit.TypeOf[T](), Value: v})
		if err != nil {
			return nil, err
		}
		vs[i] = data
	}

	return json.Marshal(vs)
}

func (ary *Array[T]) UnmarshalJSON(data []byte) error {
	if !isInterfaceType[T]() {
		v := []T(*zerokit.Coalesce(ary, &Array[T]{}))
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*ary = v
		return nil
	}

	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}

	var vs = make(Array[T], len(raws))
	for i, data := range raws {
		var tv = typed{Type: reflectkit.TypeOf[T]()}
		if err := json.Unmarshal(data, &tv); err != nil {
			return err
		}
		v, err := typeAssert[T](tv.Value)
		if err != nil {
			return err
		}
		vs[i] = v
	}

	*ary = vs
	return nil
}

func isInterfaceType[T any]() bool {
	return reflectkit.TypeOf[T]().Kind() == reflect.Interface
}

type TypeID string

func (id TypeID) String() string { return string(id) }

func (id TypeID) IsZero() bool {
	const zero TypeID = ""
	return id == zero
}

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
	for i := 0; i < 42; i++ { // max pointer to pointer nesting level is limited to 42 levels
		if check(ptr.Type().Elem()) {
			return ptr, nil
		}
		ptrPtr := reflect.New(ptr.Type())
		ptrPtr.Elem().Set(ptr)
		ptr = ptrPtr
	}
	const format = "unable to find implementation for %s with %s"
	return reflect.Value{}, fmt.Errorf(format, targetType.String(), identifiedType.String())
}

func isPrimitive(data []byte) bool {
	return isString(data) ||
		isNumber(data) ||
		isBoolean(data)
}

func isString(data []byte) bool {
	// A sequence of characters, typically used to represent text.
	// Strings in JSON must be enclosed in double quotes (").
	return bytes.HasPrefix(data, quote) && bytes.HasSuffix(data, quote)
}

func isNumber(data []byte) bool {
	// Represents both integer and floating-point numbers.
	// JSON does not distinguish between different numeric types (e.g., int, float).
	// There's no format for infinity or NaN (Not-a-Number) values in JSON.
	var n json.Number
	err := json.Unmarshal(data, &n)
	return err == nil
}

func isObject(data []byte) bool {
	// A collection of key/value pairs where the keys are strings,
	// and the values can be any JSON type.
	// Objects in JSON are similar to dictionaries in Python, hashes in Ruby, or maps in Go and Java.
	// They are enclosed in curly braces ({}).
	return bytes.HasPrefix(data, curlyBracketOpen) &&
		bytes.HasSuffix(data, curlyBracketClose)
}

func isArray(data []byte) bool {
	// An ordered list of values, which can be of any JSON type.
	// Arrays are enclosed in square brackets ([]).
	return bytes.HasPrefix(data, bracketOpen) &&
		bytes.HasSuffix(data, bracketClose)
}

func isBoolean(data []byte) bool {
	// Represents a true or false value.
	return bytes.Equal(data, trueSym) ||
		bytes.Equal(data, falseSym)
}

func isNull(data []byte) bool {
	// Represents a null value, indicating the absence of any value.
	return bytes.Equal(data, null)
}

const ErrNotInterfaceType errorkit.Error = "jsondto.ErrNotInterfaceType"

type Interface[I any] struct{ V I }

func (i Interface[I]) MarshalJSON() ([]byte, error) {
	if !isInterfaceType[I]() {
		return nil, ErrNotInterfaceType
	}
	return json.Marshal(typed{
		Type:  reflectkit.TypeOf[I](),
		Value: i.V,
		Force: true,
	})
}

func (i *Interface[I]) UnmarshalJSON(data []byte) error {
	if !isInterfaceType[I]() {
		return ErrNotInterfaceType
	}
	var t = typed{
		Type:  reflectkit.TypeOf[I](),
		Value: nil,
		Force: true,
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	value, err := typeAssert[I](t.Value)
	if err != nil {
		return fmt.Errorf("unexpected value unmarshaled (%w): %#v", err, t.Value)
	}
	*i = Interface[I]{V: value}
	return nil
}

func typeAssert[T any](v any) (T, error) {
	value, ok := v.(T)
	if !ok && !isInterfaceType[T]() {
		return *new(T), fmt.Errorf("type assertion failed, expected %s got %T",
			reflectkit.TypeOf[T]().String(), v)
	}
	return value, nil
}
