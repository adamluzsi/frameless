package codec

// Quick Reference:
// ┌──────────────┬──────────────────────────---┬─────────────────────────┐
// │ Operation    │ Input/Output                │ Best For                │
// ├──────────────┼─────────────────────────--─-┼─────────────────────────┤
// │ Marshal      │ Value     → []byte (memory) │ Complete serialization  │
// │ Unmarshal    │ []byte    → Value (memory)  │ Complete deserialization│
// │ Encode       │ Value...  → io.Writer       │ Streaming output        │
// │ Decode       │ io.Reader → Value...        │ Streaming input         │
// └──────────────┴────────────────────────---──┴─────────────────────────┘
//

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

// ListMarshaler provides slice-specific marshaling functionality.
// It's a specialized interface for encoding slices (lists) of values into byte representations.
type ListMarshaler[S ~[]E, E any] interface {
	// MarshalList encodes a slice of values into its byte representation.
	//
	// The entire slice is encoded atomically. If the operation fails, an error
	// is returned and any partial output should be considered invalid.
	//
	// Implementations may optimize the encoding for lists, potentially using
	// more compact representations than individual element marshaling.
	MarshalList(vs S) ([]byte, error)
}

// ListUnmarshaler provides slice-specific unmarshaling functionality.
// It's a specialized interface for decoding byte representations back into slices,
// complementing ListMarshaler.
type ListUnmarshaler[S ~[]E, E any] interface {
	// UnmarshalList decodes byte data into a slice of values.
	//
	// The pointer p must be to a valid slice (not nil), which will be
	// replaced with the decoded data. The entire byte slice is consumed.
	//
	// If the operation fails, the state of p is undefined and should be
	// considered invalid.
	UnmarshalList(data []byte, p *[]E) error
}
