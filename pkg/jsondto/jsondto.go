package jsondto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/pkg/zerokit"
	"reflect"
	"sync"
)

type Array[T any] []T

var ( // Symbols
	null              = []byte("null")
	curlyBracketOpen  = []byte("{")
	curlyBracketClose = []byte("}")
	fieldSeparator    = []byte(",")
)

func (ary *Array[T]) UnmarshalJSON(data []byte) error {
	if !ary.isInterfaceType() {
		v := []T(*zerokit.Coalesce(ary, &Array[T]{}))
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*ary = v
		return nil
	}

	interfaceType := reflect.TypeOf((*T)(nil)).Elem()

	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return err
	}

	type Probe struct {
		TypeID *TypeID `json:"__type"`
	}

	var vs = make(Array[T], len(raws))

parsing:
	for i, data := range raws {
		if bytes.Equal(data, null) {
			if err := json.Unmarshal(data, &vs[i]); err != nil {
				return err
			}
			continue parsing
		}

		var p Probe
		if err := json.Unmarshal(data, &p); err != nil {
			return err
		}

		if p.TypeID == nil {
			const fmtErrUnknownType = "unable to identify the %s implementation type for element %d:\n%s"
			return fmt.Errorf(fmtErrUnknownType, interfaceType.String(), i, string(data))
		}

		rType, ok := registry.TypeByID(*p.TypeID)
		if !ok {
			return fmt.Errorf("%s is not a registered type identifier", *p.TypeID)
		}

		ptr, err := newImpl(interfaceType, rType)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(data, ptr.Interface()); err != nil {
			return err
		}

		vT := reflect.New(interfaceType)
		vT.Elem().Set(ptr.Elem())
		vs[i] = vT.Elem().Interface().(T)
	}

	*ary = vs
	return nil
}

func (ary Array[T]) MarshalJSON() ([]byte, error) {
	if !ary.isInterfaceType() {
		return json.Marshal([]T(ary))
	}

	var vs []json.RawMessage
	for i, v := range ary {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}

		switch {
		case bytes.HasPrefix(data, curlyBracketOpen):
			data = bytes.TrimPrefix(data, curlyBracketOpen)
			if !bytes.HasPrefix(data, curlyBracketClose) {
				data = append(append([]byte{}, fieldSeparator...), data...)
			}

			typeID, ok := registry.TypeIDFor(v)
			if !ok {
				return nil, fmt.Errorf("missing __type id alias for %T", v)
			}

			typeIDPart, err := json.Marshal(map[string]TypeID{__type: typeID})
			if err != nil {
				return nil, err
			}
			typeIDPart = bytes.TrimSuffix(typeIDPart, curlyBracketClose)
			data = append(append([]byte{}, typeIDPart...), data...)
		}

		if !json.Valid(data) {
			return nil, fmt.Errorf("json marshaling failed for %d element:\n%s",
				i, string(data))
		}

		vs = append(vs, data)
	}

	return json.Marshal(vs)
}

func (ary Array[T]) isInterfaceType() bool {
	return reflect.TypeOf((*T)(nil)).Elem().Kind() == reflect.Interface
}

type TypeID string

const __type = `__type`

var registry __Registry

func Register[T any](id TypeID) func() {
	rType := reflect.TypeOf((*T)(nil)).Elem()
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

func newImpl(interfaceType, valueType reflect.Type) (reflect.Value, error) {
	ptr := reflect.New(valueType)
	for i := 0; i < 42; i++ { // max pointer to pointer nesting level is limited to 42 levels
		if ptr.Type().Elem().Implements(interfaceType) {
			return ptr, nil
		}
		ptrPtr := reflect.New(ptr.Type())
		ptrPtr.Elem().Set(ptr)
		ptr = ptrPtr
	}
	const format = "unable to find implementation for %s with %s"
	return reflect.Value{}, fmt.Errorf(format, interfaceType.String(), valueType.String())
}
