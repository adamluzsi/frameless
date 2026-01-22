package codec

import (
	"io"
	"iter"
)

// Codec defines codec implementation for a given format and a given type.
// It combines both marshaling (encoding to bytes) and unmarshaling (decoding from bytes)
// capabilities into a single interface, useful for bidirectional serialization.
type Codec[T any] interface {
	MarshalerT[T]
	UnmarshalerT[T]
}

// MarshalerT[T] encodes values into byte slices.
// Marshal is a stateless operation that converts an entire value into a complete byte slice.
// This is ideal for:
//   - One-off encoding operations
//   - Complete serialization to memory
//   - Situations where you have the full value upfront
//   - Formats like JSON, Protocol Buffers, or MessagePack
//
// Example use cases:
//   - json.Marshal(v) - returns complete JSON bytes
//   - Encoding database rows for storage
//   - Creating fixed-size message payloads
type MarshalerT[T any] interface {
	// MarshalT encodes a value v into a complete byte slice.
	// It returns the encoded bytes and any error encountered during encoding.
	// The operation is atomic: either the entire value is successfully encoded,
	// or an error is returned and the output is meaningless.
	MarshalT(v T) ([]byte, error)
}

// UnmarshalerT[T] decodes byte slices into values.
// Unmarshal is a stateless operation that converts a complete byte slice into a value.
// This is ideal for:
//   - One-off decoding operations
//   - Complete deserialization from memory
//   - Situations where you have all bytes available
//   - Formats like JSON, Protocol Buffers, or MessagePack
//
// Example use cases:
//   - json.Unmarshal(data, &v) - parses complete JSON bytes
//   - Loading database rows from storage
//   - Parsing fixed-size message payloads
type UnmarshalerT[T any] interface {
	// UnmarshalT decodes a byte slice into a value of type T.
	// It returns the decoded value and any error encountered during decoding.
	// The entire byte slice must be consumed; partial data may result in errors
	// or incomplete values depending on the format.
	UnmarshalT(data []byte, p *T) error
}

// EncoderT encodes a value of type T into its serialized representation.
//
// EncoderT represents a specific encoding format's implementation, encapsulating
// the serialization logic and format-specific details. Rather than returning
// serialized bytes (as Marshal functions do), EncoderT writes the encoded data
// to an underlying destination managed by the encoder itself.
//
// The destination and encoding format are determined by the concrete EncoderT
// implementation and might be opaque to the caller.
//
// Implementations should:
//   - Serialize the provided value v according to the encoding format
//   - Write or store the encoded data through the encoder's internal mechanism
//   - Return an error if encoding fails or if writing to the destination fails
//
// Typical usage involves obtaining an EncoderT instance for a specific format
// and destination, then calling Encode to process values.
//
// EncoderT differs from Marshal-style functions which return serialized bytes
// as return values.
type EncoderT[T any] interface {
	EncodeT(v T) error
}

// DecoderT decodes data in a specific serialized format into a value of type T.
//
// DecoderT represents a specific decoding format's implementation, encapsulating
// the deserialization logic and format-specific details. Rather than accepting
// serialized bytes as a parameter (as Unmarshal functions do), DecoderT reads
// serialized data from an underlying source managed by the decoder itself.
//
// The source and decoding format are determined by the concrete DecoderT
// implementation and are opaque to the caller.
//
// Implementations should:
//   - Read or retrieve serialized data from the decoder's internal source
//   - Deserialize the data according to the decoding format
//   - Populate the value pointed to by p with the deserialized data
//   - Return an error if decoding fails or if reading from the source fails
//
// The pointer parameter p must not be nil. Implementations are not responsible
// for validating this, but callers must ensure a valid pointer is provided.
//
// Typical usage involves obtaining a DecoderT instance for a specific format
// and source, then calling Decode to process values.
//
// DecoderT differs from Unmarshal-style functions which accept serialized bytes as parameters,
// as it encapsulates already of how the source data is acquired and serialised.
type DecoderT[T any] interface {
	DecodeT(p *T) error
}

type StreamProducerT[T any] interface {
	NewStreamEncoderT(w io.Writer) StreamEncoderT[T]
}

type StreamConsumerT[T any] interface {
	NewStreamDecoderT(r io.Reader) StreamDecoderT[T]
}

// StreamEncoderT encodes multiple values of type T.
//
// StreamEncoderT implements the EncoderT[T] interface for encoding values in sequence.
// It uses io.Closer to leave room in the implementation to flush or finalize any pending operations.
// This enables optimizations such as buffering or batching of encoded values,
// where the final Close call ensures all accumulated data is properly written or finalized.
//
// Implementations should:
//   - Implement Encoder[T], encoding values according to the format
//   - Implement io.Closer to finalize pending operations
//   - Return an error from Close if finalization or resource cleanup fails
//
// Typical usage involves obtaining a StreamEncoderT, encoding one or more values,
// and then closing it to finalize any pending operations.
type StreamEncoderT[T any] interface {
	EncoderT[T]
	io.Closer
}

// StreamDecoderT decodes multiple values of type T sequentially as an iterator.
//
// StreamDecoderT is an iterator that yields Decoder[T] instances along with any stream associated error.
// Each iteration yields a Decoder for the next value and or the error that the stream processing encountered.
// This allows callers to decode multiple values in sequence using range loops.
//
// The iterator abstraction provides natural control flow for processing multiple
// encoded values without explicit loop management.
type StreamDecoderT[T any] = iter.Seq2[DecoderT[T], error]
