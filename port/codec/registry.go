package codec

import (
	"go.llib.dev/frameless/internal/errorkitlite"
)

// Registry defines the typeles common codec, which should have the ability to marshal/unmarshal various types,
// or to be used as part of a codec set where using gramatically not possible to express a dynamic set of supported type
type Registry interface {
	// Marshal encodes a value v into a byte slice.
	Marshal(v any) ([]byte, error)
	// Unmarshal decodes a byte slice into a provided pointer ptr.
	Unmarshal(data []byte, ptr any) error
}

const ErrNotSupported errorkitlite.Error = "ErrNotSupported"

func NewRegistry() Registry {
	return (*nullRegistry)(nil)
}

func Register[T any](r Registry, c Codec[T]) Registry {
	if r == nil {
		r = (*nullRegistry)(nil)
	}
	return reg{
		M: func(v any) ([]byte, error) {
			if v, ok := v.(T); ok {
				return c.Marshal(v)
			}
			return r.Marshal(v)
		},
		U: func(data []byte, ptr any) error {
			if ptr, ok := ptr.(*T); ok {
				return c.Unmarshal(data, ptr)
			}
			return r.Unmarshal(data, ptr)
		},
	}
}

type nullRegistry struct{}

func (*nullRegistry) Marshal(v any) ([]byte, error) {
	return nil, ErrNotSupported
}

func (*nullRegistry) Unmarshal(data []byte, ptr any) error {
	return ErrNotSupported
}

var _ Registry = reg{}

type reg struct {
	S func(v any) bool
	M MarshalFunc[any]
	U func(data []byte, ptr any) error
}

func (i reg) Supports(v any) bool                  { return i.S(v) }
func (i reg) Marshal(v any) ([]byte, error)        { return i.M(v) }
func (i reg) Unmarshal(data []byte, ptr any) error { return i.U(data, ptr) }
