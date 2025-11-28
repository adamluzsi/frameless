package codec

import "fmt"

var _ CodecT[int] = ImplementT[int]{}

type ImplementT[T any] struct {
	MarshalFunc   func(v T) ([]byte, error)
	UnmarshalFunc func(data []byte) (T, error)
}

func (c ImplementT[T]) Marshal(v T) ([]byte, error) {
	return c.MarshalFunc(v)
}

func (c ImplementT[T]) Unmarshal(data []byte) (T, error) {
	return c.UnmarshalFunc(data)
}

func (c ImplementT[T]) CodecG() CodecG {
	return ImplementG{
		SupportsFunc: func(v any) bool {
			_, ok := v.(T)
			return ok
		},
		MarshalFunc: func(v any) ([]byte, error) {
			return c.MarshalFunc(v.(T))
		},
		UnmarshalFunc: func(data []byte, ptr any) error {
			p, ok := ptr.(*T)
			if !ok {
				return fmt.Errorf("type mismatch, expected %T but got %T", (*T)(nil), ptr)
			}
			v, err := c.UnmarshalFunc(data)
			*p = v
			return err
		},
	}
}

var _ CodecG = ImplementG{}

type ImplementG struct {
	SupportsFunc  func(v any) bool
	MarshalFunc   func(v any) ([]byte, error)
	UnmarshalFunc func(data []byte, ptr any) error
}

func (i ImplementG) Supports(v any) bool                  { return i.SupportsFunc(v) }
func (i ImplementG) Marshal(v any) ([]byte, error)        { return i.MarshalFunc(v) }
func (i ImplementG) Unmarshal(data []byte, ptr any) error { return i.UnmarshalFunc(data, ptr) }
