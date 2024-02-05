package jsondto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"reflect"
	"sync"
)

var ( // Symbols
	null              = []byte("null")
	trueSym           = []byte("true")
	falseSym          = []byte("false")
	fieldSep          = []byte(",")
	quote             = []byte(`"`)
	bracketOpen       = []byte("[")
	bracketClose      = []byte("]")
	curlyBracketOpen  = []byte("{")
	curlyBracketClose = []byte("}")
)

type Typed[T any] struct{ V T }

func (v Typed[T]) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(v.V)
	if err != nil {
		return nil, err
	}

	if !isInterfaceType[T]() {
		switch {
		case isNull(data), isPrimitive(data):
			return data, nil
		}
	}

	switch {
	case isObject(data) && isInterfaceType[T]():
		data = bytes.TrimPrefix(data, curlyBracketOpen)
		if !bytes.HasPrefix(data, curlyBracketClose) {
			data = append(append([]byte{}, fieldSep...), data...)
		}

		typeID, ok := registry.TypeIDFor(v.V)
		if !ok {
			return nil, fmt.Errorf("missing __type id alias for %T", v.V)
		}
		typeIDPart, err := json.Marshal(map[string]TypeID{__type: typeID})
		if err != nil {
			return nil, err
		}
		typeIDPart = bytes.TrimSuffix(typeIDPart, curlyBracketClose)
		data = append(append([]byte{}, typeIDPart...), data...)

	}

	if !json.Valid(data) {
		return nil, fmt.Errorf("json marshaling failed for %T.\n%s",
			v.V, string(data))
	}

	return data, nil
}

func (v *Typed[T]) UnmarshalJSON(data []byte) error {
	if !isInterfaceType[T]() {
		switch {
		case isNull(data), isPrimitive(data):
			var val Typed[T]
			if err := json.Unmarshal(data, &val.V); err != nil {
				return err
			}
			*v = val
			return nil
		}
	}

	if isObject(data) {
		return v.unmarshalObject(data)
	}

	var val Typed[T]
	if err := json.Unmarshal(data, &val.V); err != nil {
		return err
	}
	*v = val
	return nil
}

func (v *Typed[T]) unmarshalObject(data []byte) error {
	targetType := reflectkit.TypeOf[T]()

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

	rType, ok := registry.TypeByID(*p.TypeID)
	if !ok {
		return fmt.Errorf("%s is not a registered type identifier", *p.TypeID)
	}

	ptr, err := newImpl(targetType, rType)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return err
	}

	vT := reflect.New(targetType)
	vT.Elem().Set(ptr.Elem())
	v.V = vT.Elem().Interface().(T)
	return nil
}

type Array[T any] []T

func (ary Array[T]) MarshalJSON() ([]byte, error) {
	if !isInterfaceType[T]() {
		return json.Marshal([]T(ary))
	}

	var vs = make([]json.RawMessage, len(ary))
	for i, v := range ary {
		data, err := json.Marshal(Typed[T]{V: v})
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
		var tv Typed[T]
		if err := json.Unmarshal(data, &tv); err != nil {
			return err
		}
		vs[i] = tv.V
	}

	*ary = vs
	return nil
}

func isInterfaceType[T any]() bool {
	return reflectkit.TypeOf[T]().Kind() == reflect.Interface
}

type TypeID string

const __type = `__type`

var registry __Registry

func Register[T any](id TypeID) func() {
	rType := reflectkit.TypeOf[T]()
	registry.RegisterType(rType, id)
	return func() { registry.UnregisterType(rType, id) }
}

type __Registry struct {
	lock          sync.RWMutex
	_TypeIDByType map[reflect.Type]TypeID
	_TypeByTypeID map[TypeID]reflect.Type
}

func (r *__Registry) Init() {
	registry.lock.RLock()
	ok := registry._TypeIDByType != nil && registry._TypeByTypeID != nil
	registry.lock.RUnlock()
	if ok {
		return
	}
	registry.lock.Lock()
	defer registry.lock.Unlock()
	registry._TypeIDByType = make(map[reflect.Type]TypeID)
	registry._TypeByTypeID = make(map[TypeID]reflect.Type)
}

func (r *__Registry) RegisterType(rType reflect.Type, id TypeID) {
	r.Init()
	r.lock.Lock()
	defer r.lock.Unlock()
	rType = r.base(rType)
	gotID, ok := registry.typeIDByType(rType)
	if ok {
		const unableToRegisterFormat = "Unable to register %s __type id for %s, because it is already registered with %s"
		panic(fmt.Sprintf(unableToRegisterFormat, id, rType.String(), gotID))
	}
	r._TypeIDByType[rType] = id
	r._TypeByTypeID[id] = rType
}

func (r *__Registry) UnregisterType(rType reflect.Type, id TypeID) {
	r.Init()
	r.lock.Lock()
	defer r.lock.Unlock()
	rType = r.base(rType)
	delete(r._TypeIDByType, rType)
	delete(r._TypeByTypeID, id)
}

func (r *__Registry) TypeIDFor(v any) (TypeID, bool) {
	return r.TypeIDByType(reflect.TypeOf(v))
}

func (r *__Registry) TypeIDByType(typ reflect.Type) (TypeID, bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.typeIDByType(typ)
}

func (r *__Registry) typeIDByType(typ reflect.Type) (TypeID, bool) {
	if typ == nil {
		return "", false
	}
	if r._TypeIDByType == nil {
		return *new(TypeID), false
	}
	id, ok := r._TypeIDByType[r.base(typ)]
	return id, ok
}

func (r *__Registry) base(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

func (r *__Registry) TypeByID(id TypeID) (reflect.Type, bool) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	if r._TypeByTypeID == nil {
		return nil, false
	}
	rType, ok := r._TypeByTypeID[id]
	return rType, ok
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
