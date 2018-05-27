package frameless

import (
	"context"
	"io"
)

// Controller defines how a framework independent controller should look
//  the core concept that the controller implements the business rules
// 	and the channels like CLI or HTTP only provide data to it in a form of application specific business entities
type Controller interface {
	Serve(Presenter, Request) error
}

// ControllerFunc Wrapper type to convert anonimus lambda expressions into valid Frameless Controller object
type ControllerFunc func(Presenter, Request) error

// Serve func implements the Frameless Controller interface
func (lambda ControllerFunc) Serve(p Presenter, r Request) error {
	return lambda(p, r)
}

// Fetcher is an object that implements fetching logic for a given Business Entity
type Fetcher interface {
	// Return an Iterator that provides all possible value that the given Fetcher able to locate
	All() Iterator
	// Return a Fetcher that memorized the Filter requirements
	Filter(query map[string]interface{}) Fetcher
}

// Getter interface allows to look up one specific object from a given data pile*
type Getter interface {
	// Get gets the first value associated with the given key.
	// By convention it should be a single value
	Get(key interface{}) interface{}

	// Lookup gets the first value associated with the given key.
	// If there are no values associated with the key, Get returns a second value FALSE.
	Lookup(key interface{}) (interface{}, bool)
}

// Iterator will provide data to the user in a stream like way
//
// inspirated by https://golang.org/pkg/encoding/json/#Decoder
type Iterator interface {
	// this is required to make it able to cancel iterators where resource being used behind the schene
	// 	for all other case where the underling io is handled on higher level, it should simply return nil
	io.Closer
	// More can tell if there is still more value left or not
	More() bool
	// Err return the cause if for some reason by default the More return false all the time
	Err() error
	// Decode will populate an object with values and/or return error
	Decode(interface{}) error
}

// IteratorBuilder is a generic example for building iterators how should look
type IteratorBuilder func(io.ReadCloser) Iterator

// Presenter is represent a communication layer presenting layer
//
// Scope:
// 	receive messages, and convert it into a serialized form
//
// You should not allow the users of the Presenter object to modify the state of the enwrapped communication channel, such as closing, or direct writing
type Presenter interface {
	//
	// RenderWithTemplate a content on a channel that the Presenter implements
	//	name helps to determine the what template should be used, but should not include channel specific names
	//	data is the content that should be used in the template
	// RenderWithTemplate(name string, data frameless.Content) error

	//
	// Render renders a simple message back to the enwrapped communication channel
	//	message is an interface type because the channel communication layer and content and the serialization is up to the Presenter to implement
	Render(message interface{}) error
}

// PresenterFunc is a wrapper to convert standalone functions into a presenter
type PresenterFunc func(interface{}) error

// PresenterBuilder is an example how presenter should be created
type PresenterBuilder func(io.Writer) Presenter

// Render implements the Presenter Interface
func (lambda PresenterFunc) Render(message interface{}) error {
	return lambda(message)
}

// Request is framework independent way of interacting with a request that has been received on some kind of channel.
// 	from this, the controller should get all the data and options that should required for the business rule processing
type Request interface {
	io.Closer
	Context() context.Context
	Options() Getter
	Data() Iterator
}
