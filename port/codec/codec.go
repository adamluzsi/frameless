package codec

// Quick Reference:
// ┌──────────────┬──────────────────────────┬─────────────────────────┐
// │ Operation    │ Input/Output             │ Best For                │
// ├──────────────┼──────────────────────────┼─────────────────────────┤
// │ Marshal      │ Value → []byte (memory)  │ Complete serialization  │
// │ Unmarshal    │ []byte → Value (memory)  │ Complete deserialization│
// │ Encode       │ Value → io.Writer        │ Streaming output        │
// │ Decode       │ io.Reader → Value        │ Streaming input         │
// └──────────────┴──────────────────────────┴─────────────────────────┘

// Codec defines codec implementation for a given format and a given type.
// It combines both marshaling (encoding to bytes) and unmarshaling (decoding from bytes)
// capabilities into a single interface, useful for bidirectional serialization.
type Codec[T any] interface {
	Marshaler[T]
	Unmarshaler[T]
}

// Marshaler[T] encodes values into byte slices.
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
type Marshaler[T any] interface {
	// Marshal encodes a value v into a complete byte slice.
	// It returns the encoded bytes and any error encountered during encoding.
	// The operation is atomic: either the entire value is successfully encoded,
	// or an error is returned and the output is meaningless.
	Marshal(v T) ([]byte, error)
}

// Unmarshaler[T] decodes byte slices into values.
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
type Unmarshaler[T any] interface {
	// Unmarshal decodes a byte slice into a value of type T.
	// It returns the decoded value and any error encountered during decoding.
	// The entire byte slice must be consumed; partial data may result in errors
	// or incomplete values depending on the format.
	Unmarshal(data []byte, p *T) error
}
