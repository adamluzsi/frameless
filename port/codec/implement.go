package codec

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
