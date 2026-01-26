package codec

import (
	"io"
	"iter"
)

// Codec defines codec implementation for a given format and a given type.
type Codec[T any] interface {
	TypeMarshaler[T]
	TypeUnmarshaler[T]
}

type TypeMarshaler[T any] interface {
	Marshal(v T) ([]byte, error)
}

type TypeUnmarshaler[T any] interface {
	Unmarshal(data []byte, p *T) error
}

type TypeEncoder[T any] interface {
	Encode(v T) error
}

type TypeDecoder[T any] interface {
	Decode(p *T) error
}

type TypeStreamProducer[T any] interface {
	NewStreamEncoder(w io.Writer) TypeStreamEncoder[T]
}

type TypeStreamConsumer[T any] interface {
	NewStreamDecoder(r io.Reader) TypeStreamDecoder[T]
}

type TypeStreamEncoder[T any] interface {
	TypeEncoder[T]
	io.Closer
}

type TypeStreamDecoder[T any] = iter.Seq2[TypeDecoder[T], error]
