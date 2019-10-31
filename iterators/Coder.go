package iterators

import "github.com/adamluzsi/frameless"

// Encoder is a scope isolation boundary.
// One use-case for this is for example the Presenter object that encapsulate the external resource presentation mechanism from it's user.
//
// Scope:
// 	receive Entities, that will be used by the creator of the Encoder
type Encoder interface {
	//
	// Encode encode a simple message back to the wrapped communication channel
	//	message is an interface type because the channel communication layer and content and the serialization is up to the Encoder to implement
	//
	// If the message is a complex type that has multiple fields,
	// an exported struct that represent the content must be declared at the controller level
	// and all the presenters must based on that input for they test
	Encode(frameless.Entity) error
}

// EncoderFunc is a wrapper to convert standalone functions into a presenter
type EncoderFunc func(interface{}) error

// Encode implements the Encoder Interface
func (lambda EncoderFunc) Encode(i interface{}) error {
	return lambda(i)
}

// Decoder is the interface for populating/replacing a public struct with values that retried from an external resource
type Decoder interface {
	// Decode will populate an object with values and/or return error
	Decode(ptr interface{}) error
}

// DecoderFunc enables to use anonymous functions to be a valid DecoderFunc
type DecoderFunc func(interface{}) error

// Decode proxy the call to the wrapped Decoder function
func (lambda DecoderFunc) Decode(i interface{}) error {
	return lambda(i)
}
