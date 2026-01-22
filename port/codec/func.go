package codec

type MarshalFunc func(v any) ([]byte, error)

func (fn MarshalFunc) Marshal(v any) ([]byte, error) {
	return fn(v)
}

type UnmarshalFunc func(data []byte, ptr any) error

func (fn UnmarshalFunc) Unmarshal(data []byte, ptr any) error {
	return fn(data, ptr)
}

// MarshalTFunc is the marshaling implementation
type MarshalTFunc[T any] func(v T) ([]byte, error)

func (fn MarshalTFunc[T]) Marshal(v any) ([]byte, error) {
	vT, ok := v.(T)
	if !ok {
		return nil, ErrNotSupported{}
	}
	return fn(vT)
}

func (fn MarshalTFunc[T]) MarshalT(v T) ([]byte, error) {
	return fn(v)
}

type UnmarshalTFunc[T any] func(data []byte, p *T) error

func (fn UnmarshalTFunc[T]) Unmarshal(data []byte, ptr any) error {
	p, ok := ptr.(*T)
	if !ok {
		return ErrNotSupported{}
	}
	return fn(data, p)
}

func (fn UnmarshalTFunc[T]) UnmarshalT(data []byte, p *T) error {
	return fn(data, p)
}

type EncoderFunc[T any] func(v T) error

func (fn EncoderFunc[T]) Encode(v any) error {
	vT, ok := v.(T)
	if !ok {
		return ErrNotSupported{}
	}
	return fn(vT)
}

func (fn EncoderFunc[T]) EncodeT(v T) error {
	return fn(v)
}

type DecoderFunc[T any] func(p *T) error

func (fn DecoderFunc[T]) Decode(ptr any) error {
	p, ok := ptr.(*T)
	if !ok {
		return ErrNotSupported{}
	}
	return fn(p)
}

func (fn DecoderFunc[T]) DecodeT(p *T) error {
	return fn(p)
}
