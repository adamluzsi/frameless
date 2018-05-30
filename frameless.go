// Package frameless aims to create convention to build software from ground instead of upside down.
//
// But what does this in in practice? This package makes you be less productive in speed for short term! :)
//
// The goal of the package is the learning and practicing [The Clean Architecture](https://8thlight.com/blog/uncle-bob/2012/08/13/the-clean-architecture.html).
// By doing so you will create web/cli/{{ .ChannelName }} applications that actually external channel and framework independent.
//
// By working disciplined and separate the scope of your code and enforcing [law of demeter](https://en.wikipedia.org/wiki/Law_of_Demeter) on your architecture,
// by my you probably end up with something that is boring in term of code, not have fancy.
//
// The results will be something like:
//
//  * boring code
//  * separation between
//    * external interface
//    * external resource
//    * template layer
//    * presentation & serialization
//    * application control logic
//      * use cases
//      * controllers
//    * Business entities
//
// *Yes but how will this help me to achieve this ?*
//
// Basically because because the overwhelming possibility to what technology use for a project,
// sometimes these days reverse the traditional development from ground up to upside down way.
// So instead of starting to create pure Business rules and business core entities,
// the developer tricked into start working from End2End through external interface point of view.
// Such example is when a developer creates an application through testing (sometimes manually) from the webpage point of view,
// or like "If I click this html button on the side bar, there should be a new record in the database with xy".
// While it has faster impact in look, usually the business rules rarely created independently from the framework and external resources such as the db.
//
// While following the ideologies presented in the project, you will create applications that will be build from ground.
// You will basically create the pure business entities, than business "use cases"/rules with them,
// and as a final move, you choose what should be the external interface (cli/mq/http/{{.Channel}}).
//
// Handling relationship between business entities
//
// Relations between business entities should be implemented by function relations and controller should not know how to query the underling structure.
// For example:
//
//		type Customer interface{
//			frameless.Persistable
//			Teams() frameless.Iterator
//		}
//
//
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
	// Next will ensure that Decode return the next item when it is executed
	Next() bool
	// Err return the cause if for some reason by default the More return false all the time
	Err() error
	// Decode will populate an object with values and/or return error
	Decode(interface{}) error
}

// Persistable defines what requirements is expected from a business entity from behavior side if it is marked as a persistable object
// This interface expected to be used in the Software Application Business Entity definitions.
type Persistable interface {
	// ID represents a string serialized identifier that could be used for finding the record next time
	// It should be a value that is allowed to be published
	ID() string
	// Save expects to fulfill the role of "Create" and "Update".
	// The underling structure that implements the interface is expected to know if it's an already stored object or not.
	Save() error
	// Delete is expected to make the object look like deleted from the controller point of view.
	// The fact that it is deleted actually or just marked as "deleted_at" is up to the implementation.
	Delete() error
}
