package codec

type MarshalerFunc func(v any) ([]byte, error)

func (fn MarshalerFunc) Marshal(v any) ([]byte, error) {
	return fn(v)
}

type UnmarshalerFunc func(data []byte, ptr any) error

func (fn UnmarshalerFunc) Unmarshal(data []byte, ptr any) error {
	return fn(data, ptr)
}

type EncoderFunc func(v any) error

func (fn EncoderFunc) Encode(v any) error {
	return fn(v)
}

type DecoderFunc func(ptr any) error

func (fn DecoderFunc) Decode(ptr any) error {
	return fn(ptr)
}

// The TypeMarshalerFunc type is an adapter to allow the use of ordinary functions as codec type Marshalers.
type TypeMarshalerFunc[T any] func(v T) ([]byte, error)

func (fn TypeMarshalerFunc[T]) Marshal(v T) ([]byte, error) {
	return fn(v)
}

type TypeUnmarshalerFunc[T any] func(data []byte, p *T) error

func (fn TypeUnmarshalerFunc[T]) Unmarshal(data []byte, p *T) error {
	return fn(data, p)
}

type TypeEncoderFunc[T any] func(v T) error

func (fn TypeEncoderFunc[T]) Encode(v T) error {
	return fn(v)
}

type TypeDecoderFunc[T any] func(p *T) error

func (fn TypeDecoderFunc[T]) Decode(p *T) error {
	return fn(p)
}
