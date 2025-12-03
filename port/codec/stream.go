package codec

import (
	"io"
	"iter"
)

// StreamEncoder encodes multiple values of type T.
//
// StreamEncoder implements the Encoder[T] interface for encoding values in sequence.
// It uses io.Closer to leave room in the implementation to flush or finalize any pending operations.
// This enables optimizations such as buffering or batching of encoded values,
// where the final Close call ensures all accumulated data is properly written or finalized.
//
// Implementations should:
//   - Implement Encoder[T], encoding values according to the format
//   - Implement io.Closer to finalize pending operations
//   - Return an error from Close if finalization or resource cleanup fails
//
// Typical usage involves obtaining a StreamEncoder, encoding one or more values,
// and then closing it to finalize any pending operations.
type StreamEncoder[T any] interface {
	Encoder[T]
	io.Closer
}

// StreamDecoder decodes multiple values of type T sequentially as an iterator.
//
// StreamDecoder is an iterator that yields Decoder[T] instances along with any stream associated error.
// Each iteration yields a Decoder for the next value and or the error that the stream processing encountered.
// This allows callers to decode multiple values in sequence using range loops.
//
// The iterator abstraction provides natural control flow for processing multiple
// encoded values without explicit loop management.
type StreamDecoder[T any] = iter.Seq2[Decoder[T], error]
