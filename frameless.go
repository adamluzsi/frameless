package frameless

import (
	"io"
	"testing"
)

/*
	Entity encapsulate the most general and high-level rules of the application.

		TL;DR:
			These structures are representing purely data related to some kind of real world entity.
			It may have high level functions that use it's own data.
			It knows about nothing else but it self only.

	This interface here is only for documentation purpose

		"An entity can be an object with methods, or it can be a set of data structures and functions"
		Robert Martin

	In enterprise environment, this or the specification of this object can be shared between applications.
	If you don’t have an enterprise, and are just writing a single application, then these entities are the business objects of the application.
	They encapsulate the most general and high-level rules.
	Entity scope must be free from anything related to other software layers implementation knowledge such as SQL or HTTP request objects.

	They are the least likely to change when something external changes.
	For example, you would not expect these objects to be affected by a change to page navigation, or security.
	No operational change to any particular application should affect the entity layer.

	By convention these structures should be placed on the top folder level of the project

*/
type Entity = interface{}

/*
	Interactor implement a business rule to a specific audience of the software.
	This interface here is only for documentation purpose.

		TL;DR:
			Using Interactor imposes discipline upon focusing on the audience who's business rule you works on.

	This can be a function or a struct of function, it's up to the implementation.
	It has to be implemented in a framework independent way.
	The function arguments should be explicit Entity structures or primitives.
	When stream of data required, use case should declare framework and technology independent data providers.

		In my future examples this data source will be fulfilled by Iterator pattern implementations.

	As an easy to follow practice that you start build your application by implementing domain use cases,
	until you have all your business logic / use case implemented.
	Your test should work only with Entity structures and primitives exclusively.

	If you cannot avoid to depend on external resources, use interface to represent they need.
	During my research, I played around multiple solutions, and the one I liked the most is the following:
	You describe in a shared specification, in a test that is Exposed and importable by the external interface specification,
	and in that you describe what is your expectation from the use-case point of view from the provided external resource,
	when you call it with a specific data structure. for more about this, read the "Query" and "Resource" type.

	Here is a definition from Robert Martin:

		The software in this layer contains application specific business rules.
		It encapsulates and implements all of the use cases of the system.
		These use cases orchestrate the flow of data to and from the entities,
		and direct those entities to use their enterprise wide business rules to achieve the goals of the use case.

		We do not expect changes in this layer to affect the entities.
		We also do not expect this layer to be affected by changes to externalises such as the database,
		the UI, or any of the common frameworks.
		This layer is isolated from such concerns.

		We do, however, expect that changes to the operation of the application will affect the use-cases and therefore the software in this layer.
		If the details of a use-case change, then some code in this layer will certainly be affected.


	When your application has all the use-case, then you decide the right external interface to expose them.

		TIP:
			If you don't know how to start, imagine that every audience category your system defines has a dedicated engineer.
			For example in case of a web-shop: Buyer, Seller, Content Manager, Application Manager, DataBaseAdministrator just to name a few.
			Each engineer work on one user story for one of the audience category. You are one of the engineers.
			Almost every other engineer on the other user stories push code really frequently (for example 1 push / min).

			How would you structure and create your code in a way that you are safe from merge conflicts ?
			How would you design your code dependency in a way that other engineers activity unlikely to affect your code ?

*/
type Interactor = interface{}

/*
	Resource is a specific implementation of an external resource that required for an Interactor.

		TL;DR:
			Resource (External) imposes discipline upon dependency inversion.
			You encapsulate all the technology specific implementations,
			as a separate structure that adapt to a predefined stable but easily extendable interface,
			so you can try anything out while frameworks and ORMs evolve,
			yet your domain rules will be keep safe from this changes.

	By removing tight integration and encapsulate it behind such a generic interface like this,
	we can freely swap the implementation and make testing much more easier for the domain rules.

	One specific Resource that received multiple implementations is the "storage",
	but the usage of these is generally applicable to any kind of external resource
	which you have to interact from use-cases or from controllers.

	While at first this may seems boilerplate and you would feel a nice ORM would do the same,
	but than you took over the responsibility of ensuring that the current ORM you choose to work with
	and its current API will be maintained and if needed, backward ported for your application.
	You may ask why not just update the project code base to the newest api in the use-cases, but then
	that update would be a violation to the dependency inversion rule,
	which defines that a change in external interfaces should never affect use-cases or Entities.

	So this interface aims to reduce interconnection between use-case layer and external interface layer.
	The first is that you define Query structures that define the behavior which needs to be implemented.

*/
type Resource interface {
	io.Closer
}

/*
Iterator define a separate object that encapsulates accessing and traversing an aggregate object.
Clients use an iterator to access and traverse an aggregate without knowing its representation (data structures).
Interface design inspirited by https://golang.org/pkg/encoding/json/#Decoder
https://en.wikipedia.org/wiki/Iterator_pattern
*/
type Iterator interface {
	// this is required to make it able to cancel iterators where resource being used behind the scene
	// for all other case where the underling io is handled on higher level, it should simply return nil
	io.Closer
	// Next will ensure that Decode return the next item when it is executed
	Next() bool
	// Err return the cause if for some reason by default the More return false all the time
	Err() error
	// Decode will populate an object with values and/or return error
	// this is required to retrieve the current value from the iterator
	Decode(Entity) error
}

/*
	Error is an implementation for the error interface that allow you to declare exported globals with the `const` keyword.

		TL;DR:
			const ErrSomething frameless.Error = "something is an error"

*/
type Error string

// Error implement the error interface
func (err Error) Error() string { return string(err) }

/*
Spec represent a shared specification that intention is to request specific behavior for commonly used components.
One example could be an external resource that used by an interactor, and you want ensure the expected behivour from the dependency,
by describing the behaviour in a shared specification.
*/
type Spec interface {
	Test(t *testing.T)
	Benchmark(b *testing.B)
}
