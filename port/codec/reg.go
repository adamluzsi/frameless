package codec

import "errors"

func Merge(b Bundle, bs ...Bundle) Bundle {
	for _, oth := range bs {
		b = merge(b, oth)
	}
	return b
}

func merge(b, oth Bundle) Bundle {
	if b == nil {
		return oth
	}
	return reg{
		MarshalerFunc: func(v any) ([]byte, error) {
			data, err := b.Marshal(v)
			if err == nil {
				return data, nil
			}
			if !errors.Is(err, errNotSupported{}) {
				return data, err
			}
			return oth.Marshal(v)
		},
		UnmarshalerFunc: func(data []byte, ptr any) error {
			err := b.Unmarshal(data, ptr)
			if err == nil {
				return nil
			}
			if !errors.Is(err, errNotSupported{}) {
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
		MarshalerFunc: func(v any) ([]byte, error) {
			if v, ok := v.(T); ok {
				return c.Marshal(v)
			}
			return b.Marshal(v)
		},
		UnmarshalerFunc: func(data []byte, ptr any) error {
			if ptr, ok := ptr.(*T); ok {
				return c.Unmarshal(data, ptr)
			}
			return b.Unmarshal(data, ptr)
		},
	}
}

type errNotSupported struct{}

func (errNotSupported) Error() string {
	return "ErrNotSupported"
}

type nullBundle struct{}

func (*nullBundle) Marshal(v any) ([]byte, error) {
	return nil, errNotSupported{}
}

func (*nullBundle) Unmarshal(data []byte, ptr any) error {
	return errNotSupported{}
}

var _ Bundle = reg{}

type reg struct {
	MarshalerFunc
	UnmarshalerFunc
}
