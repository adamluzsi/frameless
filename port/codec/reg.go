package codec

import "errors"

type ErrNotSupported struct{}

func (ErrNotSupported) Error() string {
	return "ErrNotSupported"
}

func Merge(b, oth Bundle) Bundle {
	if b == nil {
		return oth
	}
	return reg{
		M: func(v any) ([]byte, error) {
			data, err := b.Marshal(v)
			if err == nil {
				return data, nil
			}
			if !errors.Is(err, ErrNotSupported{}) {
				return data, err
			}
			return oth.Marshal(v)
		},
		U: func(data []byte, ptr any) error {
			err := b.Unmarshal(data, ptr)
			if err == nil {
				return nil
			}
			if !errors.Is(err, ErrNotSupported{}) {
				return err
			}
			return oth.Unmarshal(data, ptr)
		},
	}
}

func Register[T any](b Bundle, c Codec[T]) Bundle {
	if b == nil {
		b = (*nullBundle)(nil)
	}
	return reg{
		M: func(v any) ([]byte, error) {
			if v, ok := v.(T); ok {
				return c.MarshalT(v)
			}
			return b.Marshal(v)
		},
		U: func(data []byte, ptr any) error {
			if ptr, ok := ptr.(*T); ok {
				return c.UnmarshalT(data, ptr)
			}
			return b.Unmarshal(data, ptr)
		},
	}
}

type nullBundle struct{}

func (*nullBundle) Marshal(v any) ([]byte, error) {
	return nil, ErrNotSupported{}
}

func (*nullBundle) Unmarshal(data []byte, ptr any) error {
	return ErrNotSupported{}
}

var _ Bundle = reg{}

type reg struct {
	M MarshalTFunc[any]
	U func(data []byte, ptr any) error
}

func (i reg) Marshal(v any) ([]byte, error)        { return i.M(v) }
func (i reg) Unmarshal(data []byte, ptr any) error { return i.U(data, ptr) }
