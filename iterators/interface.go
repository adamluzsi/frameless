package iterators

import "io"

// Interface define a separate object that encapsulates accessing and traversing an aggregate object.
// Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
// Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
// https://en.wikipedia.org/wiki/Iterator_pattern
type Interface interface {
	// this is required to make it able to cancel iterators where resource being used behind the scene
	// for all other case where the underling io is handled on higher level, it should simply return nil
	io.Closer
	// Next will ensure that Decode return the next item when it is executed
	Next() bool
	// Err return the cause if for some reason by default the More return false all the time
	Err() error
	// Decoder will populate an object with values and/or return error
	// this is required to retrieve the current value from the iterator
	Decoder
}

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
	Encode(interface{}) error
}

// EncoderFunc is a wrapper to convert standalone functions into a presenter
type EncoderFunc func(interface{}) error

// Encode implements the Encoder Interface
func (lambda EncoderFunc) Encode(i interface{}) error {
	return lambda(i)
}

// Decoder is the interface to represent value decoding into a passed pointer type.
// Most commonly this happens with value decoding that was received from some sort of external resource.
// Decoder in other words the interface for populating/replacing a public struct with values that retried from an external resource.
type Decoder interface {
	// Decode will populate/replace/configure the value of the received pointer type
	// and in case of failure, returns an error.
	Decode(ptr interface{}) error
}

// DecoderFunc enables to use anonymous functions to be a valid DecoderFunc
type DecoderFunc func(interface{}) error

// Decode proxy the call to the wrapped Decoder function
func (lambda DecoderFunc) Decode(i interface{}) error {
	return lambda(i)
}
