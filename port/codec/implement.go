package codec

import (
	"fmt"
)

var _ Codec[int] = Implement[int]{}

type Implement[T any] struct {
	Enc func(v T) ([]byte, error)
	Dec func(data []byte, p *T) error
}

func (c Implement[T]) Marshal(v T) ([]byte, error) {
	return c.Enc(v)
}

func (c Implement[T]) Unmarshal(data []byte, p *T) error {
	return c.Dec(data, p)
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

func (c Implement[T]) Registry() Registry {
	return regrec{
		SupportsFunc: DefaultRegistrySupports[T],
		MarshalFunc: func(v any) ([]byte, error) {
			val, ok := v.(T)
			if !ok {
				return nil, fmt.Errorf("type mismatch, expected %T but got %T", val, v)
			}
			return c.Enc(val)
		},
		UnmarshalFunc: func(data []byte, ptr any) error {
			p, ok := ptr.(*T)
			if !ok {
				return fmt.Errorf("type mismatch, expected %T but got %T", (*T)(nil), ptr)
			}
			return c.Dec(data, p)
		},
	}
}
