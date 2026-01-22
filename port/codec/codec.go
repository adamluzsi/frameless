// Package codec is a port collection about codec related interactions.
//
// Quick Reference:
// ┌──────────────┬──────────────────────────---┬─────────────────────────┐
// │ Operation    │ Input/Output                │ Best For                │
// ├──────────────┼─────────────────────────--─-┼─────────────────────────┤
// │ Marshal      │ Value     → []byte (memory) │ Complete serialization  │
// │ Unmarshal    │ []byte    → Value (memory)  │ Complete deserialization│
// │ Encode       │ Value...  → io.Writer       │ Streaming output        │
// │ Decode       │ io.Reader → Value...        │ Streaming input         │
// └──────────────┴────────────────────────---──┴─────────────────────────┘
package codec

import (
	"io"
	"iter"
)

// Bundle defines the typeles common codec bundle, which should have the ability to marshal/unmarshal various types,
// or to be used as part of a codec set where using gramatically not possible to express a dynamic set of supported type
type Bundle interface {
	Marshaler
	Unmarshaler
}

type Marshaler interface {
	// Marshal encodes a value v into a byte slice.
	Marshal(v any) ([]byte, error)
}

type Unmarshaler interface {
	// Unmarshal decodes a byte slice into a provided pointer ptr.
	Unmarshal(data []byte, ptr any) error
}

type Encoder interface {
	Encode(v any) error
}

type Decoder interface {
	Decode(ptr any) error
}

type StreamProducer interface {
	NewStreamEncoder(w io.Writer) StreamEncoder
}

type StreamConsumer interface {
	NewStreamDecoder(r io.Reader) StreamDecoder
}

// StreamEncoder encodes multiple values into a stream.
//
// StreamEncoder implements the Encoder interface for encoding values in sequence.
// It uses io.Closer to leave room in the implementation to flush or finalize any pending operations.
// This enables optimizations such as buffering or batching of encoded values,
// where the final Close call ensures all accumulated data is properly written or finalized.
//
// Typical usage involves obtaining a StreamEncoder, encoding one or more values,
// and then closing it to finalize any pending operations.
type StreamEncoder interface {
	Encoder
	io.Closer
}

type StreamDecoder = iter.Seq2[Decoder, error]
