package frameless

// Decoder is the interface to represent value decoding into a passed pointer type.
// Most commonly this happens with value decoding that was received from some sort of external resource.
// Decoder in other words the interface for populating/replacing a public struct with values that retried from an external resource.
type Decoder[T any] interface {
	// Decode will populate/replace/configure the value of the received pointer type
	// and in case of failure, returns an error.
	Decode(*T) error
}

// DecoderFunc enables to use anonymous functions to be a valid DecoderFunc
type DecoderFunc[T any] func(*T) error

// Decode proxy the call to the wrapped Decoder function
func (lambda DecoderFunc[T]) Decode(ptr *T) error {
	return lambda(ptr)
}
