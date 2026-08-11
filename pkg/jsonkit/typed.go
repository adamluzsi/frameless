package jsonkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
)

type WithType[T any] struct{ V T }

var _ json.Marshaler = WithType[any]{}

func (dto WithType[T]) MarshalJSON() ([]byte, error) {
	return typed{
		Type:  reflectkit.TypeOf[T](),
		Value: dto.V,
		Force: true,
	}.MarshalJSON()
}

var _ json.Unmarshaler = (*WithType[any])(nil)

func (dto *WithType[T]) UnmarshalJSON(data []byte) error {
	var value T
	var c = typed{
		Type:  reflectkit.TypeOf[T](),
		Value: &value,
		Force: true,
	}
	if err := c.UnmarshalJSON(data); err != nil {
		return err
	}
	if c.Value != nil {
		value = c.Value.(T)
	}
	dto.V = value
	return nil
}

type WithTypeIDOf[T, IDType any] struct{ V T }

var _ json.Marshaler = WithTypeIDOf[any, any]{}

func (dto WithTypeIDOf[T, IDType]) MarshalJSON() ([]byte, error) {
	return typed{
		Type:  reflectkit.TypeOf[T](),
		Value: dto.V,
		Force: true,
	}.MarshalJSON()
}

var _ json.Unmarshaler = (*WithTypeIDOf[any, any])(nil)

func (dto *WithTypeIDOf[T, IDType]) UnmarshalJSON(data []byte) error {
	var value T
	var c = typed{
		Type:  reflectkit.TypeOf[T](),
		Value: &value,
		Force: true,
	}
	if err := c.UnmarshalJSON(data); err != nil {
		return err
	}
	if c.Value != nil {
		value = c.Value.(T)
	}
	dto.V = value
	return nil
}

type typed struct {
	Type   reflect.Type
	IDType reflect.Type
	Value  any
	Force  bool
	// reg is the codec's per-instance type registry. May be nil, in which
	// case the package-level registry is consulted.
	reg *_TypeRegistry
	// codec is the originating *Codec when typed is constructed during an
	// unmarshal dispatch. It is forwarded into the per-struct unmarshal
	// path so user-registered custom Unmarshal closures receive the same
	// codec instance that initiated the dispatch, not a freshly-zero
	// Codec that has lost its registry. May be nil for marshalling-only
	// paths or for typed values constructed outside of unmarshalReflect.
	codec *Codec
}

type typedContainer struct {
	Type  TypeID          `json:"@type"`
	Value json.RawMessage `json:"@value"`
}

// MarshalJSON will marshal Typed T value with a @type property
func (v typed) MarshalJSON() ([]byte, error) {
	const __type = `@type`

	var (
		rVal = reflect.ValueOf(v.Value)
		data []byte
		err  error
	)
	switch rVal.Kind() {
	case reflect.Slice:
		var ary jsonArray
		ary.ptr = reflectkit.PointerOf(rVal)
		ary.reg = v.reg
		data, err = json.Marshal(ary)
	case reflect.Struct:
		// If the value's concrete type brings its own json.Marshaler,
		// preserve that wire format. stdlib's json.Marshal will dispatch
		// through it; the codec's reflection path would lose any custom
		// encoding. Without its own Marshaler, route through the codec's
		// reflection-based marshalling so interface-typed fields get the
		// @type/@value envelope stdlib cannot produce.
		if rVal.CanInterface() && rVal.Type().Implements(jsonMarshalerType) {
			data, err = json.Marshal(v.Value)
		} else if entry := v.reg.LookupCustomCodec(rVal.Type()); entry != nil {
			// The concrete type has a CodecRegister-style custom
			// Marshal closure. Invoke it directly so this typed
			// envelope is the single source of the @type field —
			// recursing through marshalPlaceholderWithReg would
			// also add @type, producing a duplicate @type field on
			// the wire. The codec built here is a local shim that
			// exposes the registry to the closure; the closure is
			// not expected to retain it.
			shim := &Codec{typeIDRegistry: v.reg}
			data, err = entry.Marshal(shim, v.Value)
		} else {
			shadow, sErr := marshalPlaceholderWithReg(rVal, nil, v.reg, nil, false)
			if sErr != nil {
				err = sErr
			} else {
				data, err = json.Marshal(shadow.Interface())
			}
		}
	default:
		data, err = json.Marshal(v.Value)
	}
	if err != nil {
		return nil, err
	}

	if !isInterfaceType(v.Type) {
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
		typeID, gotTypeID = lookupTypeIDForValue(v.reg, v.Value)
	}

	switch {
	case isObject(data) && (isInterfaceType(v.Type) || v.Force):
		if v.Type.Kind() == reflect.Map {
			data, err = json.Marshal(typedContainer{
				Type:  zerokit.Coalesce(typeID, TypeID(v.Type.String())),
				Value: data,
			})
			if err != nil {
				return nil, err
			}
			break
		}
		data = bytes.TrimPrefix(data, curlyBracketOpen)
		if !bytes.HasPrefix(data, curlyBracketClose) {
			data = append(append([]byte{}, fieldSep...), data...)
		}
		if !gotTypeID {
			return nil, fmt.Errorf("missing @type id alias for %T", v.Value)
		}
		typeIDPart, err := json.Marshal(map[string]TypeID{"@type": typeID})
		if err != nil {
			return nil, err
		}
		typeIDPart = bytes.TrimSuffix(typeIDPart, curlyBracketClose)
		data = append(append([]byte{}, typeIDPart...), data...)

	case v.Force, isInterfaceType(v.Type) && gotTypeID:
		// When typed.Type is an interface (the polymorphic dispatch site),
		// the wire must carry a discriminator for non-object values too,
		// otherwise a slice/map value whose concrete type is a registered
		// polymorphic type (e.g. workflow.Sequence embedded in another
		// workflow.Sequence) would round-trip as a bare JSON array.
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

// UnmarshalJSON will deserialize the T from data.
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

func (v *typed) unmarshalObject(data []byte) error {
	var typeID TypeID

	if v.Force {
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		var tc typedContainer
		err := d.Decode(&tc)
		if err == nil {
			typeID = tc.Type
			// For a flat envelope (no @value field), keep the
			// original data intact so the registered struct's
			// custom Unmarshal can extract fields directly. Only
			// the @value variant ({"@type":"...","@value":...})
			// replaces data with the inner bytes.
			if len(tc.Value) > 0 {
				data = tc.Value
			}
		}
	}

	if typeID.IsZero() {
		var p struct {
			TypeID *TypeID `json:"@type,omitempty"`
		}

		if err := json.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("unable to unmarshal @type field for:\n%s", string(data))
		}
		if p.TypeID == nil {
			return fmt.Errorf("@type is not set")
		}
		if *p.TypeID == "" {
			return fmt.Errorf("@type is empty")
		}
		typeID = *p.TypeID
	}

	// For Force=false callers, the data was not stripped above, so a
	// typedContainer envelope would still wrap the concrete value. Strip
	// it now that we know the typeID so the dispatch below receives the
	// inner payload, not the envelope. Without this, dispatching to
	// jsonArray.UnmarshalJSON would receive `{"@type":"...","@value":[…]}`
	// where it expects a bare array and fail with "cannot unmarshal
	// object into []json.RawMessage".
	if len(data) > 0 && data[0] == '{' {
		var tc typedContainer
		if err := json.Unmarshal(data, &tc); err == nil && tc.Type == typeID && len(tc.Value) > 0 {
			data = tc.Value
		}
	}

	var value reflect.Value
	rType, ok := lookupTypeByIDForCodec(v.reg, typeID)
	if ok {
		switch rType.Kind() {
		case reflect.Slice:
			slice := reflect.MakeSlice(rType, 0, 0)
			ptr := reflect.New(rType)
			ptr.Elem().Set(slice)

			var ary jsonArray
			ary.ptr = ptr
			ary.reg = v.reg
			ary.codec = v.codec
			if err := json.Unmarshal(data, &ary); err != nil {
				return err
			}
			value = ptr.Elem()

		case reflect.Struct, reflect.Pointer:
			// Route struct values through the codec's reflection-based
			// unmarshalling so that nested interface-typed fields are
			// dispatched correctly via the per-codec registry. stdlib
			// json.Unmarshal cannot do this dispatch.
			//
			// Exception: if the concrete type brings its own
			// json.Unmarshaler, defer to its wire format so its custom
			// decoding logic is preserved.
			if rType.Implements(jsonUnmarshalerType) || reflect.PointerTo(rType).Implements(jsonUnmarshalerType) {
				ptr, err := newImpl(v.Type, rType)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(data, ptr.Interface()); err != nil {
					return err
				}
				value = ptr.Elem()
				break
			}
			ptr, err := newImpl(v.Type, rType)
			if err != nil {
				return err
			}
			// newImpl may return a pointer-to-pointer when the concrete
			// implementation sits behind a pointer (e.g. *TypeC implements
			// Greeter). Walk to the non-pointer struct before decoding.
			target := ptr
			for target.Kind() == reflect.Pointer {
				target = target.Elem()
			}
			if err := unmarshalStructWithReg(data, target, v.reg, v.codec); err != nil {
				return err
			}
			value = ptr.Elem()

		default: // try for primitives
			ptr, err := newImpl(v.Type, rType)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(data, ptr.Interface()); err != nil {
				return err
			}
			value = ptr.Elem()
		}
	} else {
		if TypeID(v.Type.String()) != typeID {
			return fmt.Errorf("%s is not a recognised primitive type", typeID)
		}
		ptr := reflect.New(v.Type)
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
