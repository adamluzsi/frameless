package codec

import (
	"reflect"

	"go.llib.dev/frameless/internal/errorkitlite"
)

// Registry defines the typeles common codec, which should have the ability to either encode/decode various types,
// or to be used as part of a codec set where using gramatically not possible to express a dynamic set of supported type
type Registry interface {
	// Supports answers whether or not this CodecG supports the provided value.
	// Supports is expected to recognise both value type and pointer types.
	// A nil value is never considered as a supported value type.
	Supports(vT any) bool
	// Marshal encodes a value v into a byte slice.
	Marshal(v any) ([]byte, error)
	// Unmarshal decodes a byte slice into a provided pointer ptr.
	Unmarshal(data []byte, ptr any) error
}

const ErrNotSupported errorkitlite.Error = "ErrNotSupported"

func MergeRegistry(rs ...Registry) Registry {
	var reg registry
	for _, r := range rs {
		if ry, ok := r.(registry); ok {
			reg = append(reg, ry...)
			continue
		}
		reg = append(reg, r)
	}
	return reg
}

var _ Registry = (registry)(nil)

type registry []Registry

func (r registry) Supports(v any) bool {
	for _, e := range r {
		if e.Supports(v) {
			return true
		}
	}
	return false
}

func (r registry) Marshal(v any) ([]byte, error) {
	for _, e := range r {
		if e.Supports(v) {
			return e.Marshal(v)
		}
	}
	return nil, ErrNotSupported
}

func (r registry) Unmarshal(data []byte, ptr any) error {
	for _, e := range r {
		if e.Supports(ptr) {
			return e.Unmarshal(data, ptr)
		}
	}
	return ErrNotSupported
}

func (r registry) deref(ptr any) (any, bool) {
	rp := reflect.ValueOf(ptr)
	if rp.Kind() != reflect.Pointer {
		return nil, false
	}
	if rp.IsNil() {
		return nil, false
	}
	return rp.Elem().Interface(), true
}

var _ Registry = regrec{}

type regrec struct {
	SupportsFunc  func(v any) bool
	MarshalFunc   func(v any) ([]byte, error)
	UnmarshalFunc func(data []byte, ptr any) error
}

func (i regrec) Supports(v any) bool                  { return i.SupportsFunc(v) }
func (i regrec) Marshal(v any) ([]byte, error)        { return i.MarshalFunc(v) }
func (i regrec) Unmarshal(data []byte, ptr any) error { return i.UnmarshalFunc(data, ptr) }
