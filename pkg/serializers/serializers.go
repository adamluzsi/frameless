package serializers

import "io"

type Serializer interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, ptr any) error
}

type ListSerializer interface {
	ListEncoderMaker
	ListDecoderMaker
}

type ListEncoderMaker interface {
	MakeListEncoder(w io.Writer) ListEncoder
}

type ListDecoderMaker interface {
	MakeListDecoder(w io.Reader) ListDecoder
}

type ListEncoder interface {
	// Encode will encode an Entity in the underlying io writer.
	Encode(v any) error
	// Closer represent the finishing of the List encoding process.
	io.Closer
}

type ListDecoder interface {
	Decode(ptr any) error
	// Next will ensure that Value returns the next item when executed.
	// If the next value is not retrievable, Next should return false and ensure Err() will return the error cause.
	Next() bool
	// Err return the error cause.
	Err() error
	// Closer is required to make it able to cancel iterators where resources are being used behind the scene
	// for all other cases where the underling io is handled on a higher level, it should simply return nil
	io.Closer
}
