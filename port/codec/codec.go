package codec

import "io"

// Codec defines the general behaviour for encoding and decoding values into a given codec format.
// An exmaple would be the JSON Codec, where values can be marshaled and unmarshaled back and forth.
type Codec interface {
	// Marshal encodes a value v into a byte slice.
	Marshal(v any) ([]byte, error)
	// Unmarshal decodes a byte slice into a provided pointer ptr.
	Unmarshal(data []byte, ptr any) error
}

// ListEncoderMaker defines an interface for creating ListEncoder instances.
// It is responsible for providing a ListEncoder that writes to a specific io.Writer.
type ListEncoderMaker interface {
	// MakeListEncoder creates a new ListEncoder that writes encoded data to the provided io.Writer.
	MakeListEncoder(w io.Writer) ListEncoder
}

// ListDecoderMaker defines an interface for creating ListDecoder instances.
// It is responsible for providing a ListDecoder that reads from a specific io.Reader.
type ListDecoderMaker interface {
	// MakeListDecoder creates a new ListDecoder that reads decoded data from the provided io.Reader.
	MakeListDecoder(w io.Reader) ListDecoder
}

// ListEncoder represents an interface for encoding multiple entities to an underlying io.Writer.
// It includes methods to encode individual values and to close the encoder once the encoding is complete.
type ListEncoder interface {
	// Encode serialises a value v and writes it to the underlying io.Writer.
	Encode(v any) error
	io.Closer
}

type ListDecoder interface {
	// Decode restores the next value from the underlying io.Reader and stores it in the provided pointer.
	Decode(ptr any) error
	// Next will ensure that Value returns the next item when executed.
	// If the next value is not retrievable, Next should return false and ensure Err() will return the error cause.
	Next() bool
	// Err return the error cause.
	Err() error
	// Closer finalises the decoding process and releases any resources held by the decoder.
	// Closer is required to make it able to cancel iterators where resources are being used behind the scene
	// for all other cases where the underling io is handled on a higher level, it should simply return nil
	io.Closer
}
