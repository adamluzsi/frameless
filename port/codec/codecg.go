package codec

import "io"

// ListEncoderG represents an interface for encoding multiple entities to an underlying io.Writer.
// It includes methods to encode individual values and to close the encoder once the encoding is complete.
type ListEncoderG interface {
	// Encode serialises a value v and writes it to the underlying io.Writer.
	Encode(v any) error
	io.Closer
}

type ListDecoderG interface {
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
