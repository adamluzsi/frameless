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

// Codec defines the typeles common codec bundle, which should have the ability to marshal/unmarshal various types,
// or to be used as part of a codec set where using gramatically not possible to express a dynamic set of supported type
type Codec interface {
	Marshaler
	Unmarshaler
}

type Marshaler interface {
	Marshal(v any) ([]byte, error)
}

type Unmarshaler interface {
	Unmarshal(data []byte, ptr any) error
}

type Encoder interface {
	Encode(v any) error
}

type Decoder interface {
	Decode(ptr any) error
}
