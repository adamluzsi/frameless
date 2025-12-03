package codec

// Encoder encodes a value of type T into its serialized representation.
//
// Encoder represents a specific encoding format's implementation, encapsulating
// the serialization logic and format-specific details. Rather than returning
// serialized bytes (as Marshal functions do), Encoder writes the encoded data
// to an underlying destination managed by the encoder itself.
//
// The destination and encoding format are determined by the concrete Encoder
// implementation and might be opaque to the caller.
//
// Implementations should:
//   - Serialize the provided value v according to the encoding format
//   - Write or store the encoded data through the encoder's internal mechanism
//   - Return an error if encoding fails or if writing to the destination fails
//
// Typical usage involves obtaining an Encoder instance for a specific format
// and destination, then calling Encode to process values.
//
// Encoder differs from Marshal-style functions which return serialized bytes
// as return values.
type Encoder[T any] interface {
	Encode(v T) error
}

// Decoder decodes data in a specific serialized format into a value of type T.
//
// Decoder represents a specific decoding format's implementation, encapsulating
// the deserialization logic and format-specific details. Rather than accepting
// serialized bytes as a parameter (as Unmarshal functions do), Decoder reads
// serialized data from an underlying source managed by the decoder itself.
//
// The source and decoding format are determined by the concrete Decoder
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
// Typical usage involves obtaining a Decoder instance for a specific format
// and source, then calling Decode to process values.
//
// Decoder differs from Unmarshal-style functions which accept serialized bytes as parameters,
// as it encapsulates already of how the source data is acquired and serialised.
type Decoder[T any] interface {
	Decode(p *T) error
}
