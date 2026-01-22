package httpcodec

import (
	"io"

	"go.llib.dev/frameless/port/codec"
)

type Codec[T any] struct {
	Marshal   codec.MarshalTFunc[T]
	Unmarshal codec.UnmarshalTFunc[T]

	List ListCodec[T]
}

type ListCodec[T any] struct {
	codec.MarshalTFunc[[]T]
	codec.UnmarshalTFunc[[]T]

	NewEncoder func(w io.Writer) codec.StreamEncoderT[T]
	NewDecoder func(r io.Reader) codec.StreamDecoderT[T]
}
