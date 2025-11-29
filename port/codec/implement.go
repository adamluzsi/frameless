package codec

import (
	"fmt"
)

// MarshalFunc is the marshaling implementation
type MarshalFunc[T any] func(v T) ([]byte, error)

var _ Marshaler[int] = (MarshalFunc[int])(nil)

func (fn MarshalFunc[T]) Marshal(v T) ([]byte, error) {
	return fn(v)
}

// UnmarshalFunc is the unmarshaling implementation
type UnmarshalFunc[T any] func(data []byte, p *T) error

var _ Unmarshaler[int] = (UnmarshalFunc[int])(nil)

func (fn UnmarshalFunc[T]) Unmarshal(data []byte, p *T) error {
	return fn(data, p)
}

var _ Codec[int] = CodecImpl[int]{}

type CodecImpl[T any] struct {
	MarshalFunc[T]
	UnmarshalFunc[T]
}

func DefaultRegistrySupports[T any](vT any) bool {
	if vT == nil {
		return false
	}
	if _, ok := vT.(T); ok {
		return true
	}
	if p, ok := vT.(*T); ok && p != nil {
		return true
	}
	return false
}

func (c CodecImpl[T]) Registry() Registry {
	return reg{
		S: DefaultRegistrySupports[T],
		M: func(v any) ([]byte, error) {
			val, ok := v.(T)
			if !ok {
				return nil, fmt.Errorf("type mismatch, expected %T but got %T", val, v)
			}
			return c.MarshalFunc(val)
		},
		U: func(data []byte, ptr any) error {
			p, ok := ptr.(*T)
			if !ok {
				return fmt.Errorf("type mismatch, expected %T but got %T", (*T)(nil), ptr)
			}
			return c.UnmarshalFunc(data, p)
		},
	}
}
