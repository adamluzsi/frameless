package frameless

import (
	"io"
)

// Iterator define a separate object that encapsulates accessing and traversing an aggregate object.
// Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
// Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
// https://en.wikipedia.org/wiki/Iterator_pattern
type Iterator interface {
	// Closer is required to make it able to cancel iterators where resource being used behind the scene
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
