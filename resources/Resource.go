package resources

import (
	"github.com/adamluzsi/frameless"
	"testing"
)

/*

	Resource are an more specific implementation, which use one single function as an entrypoint.
	and by this achi resources that implements interactions (queries) that the use case depends on.

	So this interface aims to reduce interconnection between use-case layer and external interface layer.
	The first is that you define Query structures that define the behavior which needs to be implemented.

	The design heavily inspirited by the combination of Liskov substitution principle with the dependency inversion principle.
	While the External Resource interface provides a stable abstraction, the actual implementations can fulfil the implementations with the specific technology.
	This helps to easily remove any concrete dependency from other layers by only referring to a common stable non volatile interface.

	Indeed, software designers and architects work hard to reduce the volatility of interfaces.
	They try to find ways to add functionality to implementations without making changes to the interfaces.
	This is Software Design 101.

*/
type Resource interface {
	frameless.Resource
	// Exec expect a Query object and  implements the Query#Test -s  each application Query.Test that used with the given storage.
	// This way for example the controller defines what is required in order to fulfil a business use case and storage implement that..
	//
	// for simple use cases where returned iterator expected to only include 1 element I recommend using iterators.DecodeNext(iterator, &entity) for syntax sugar.
	// The use case common but I do not see benefit enforcing a First, FindOne or similar requirement from the storage.
	// For creating iterators for a single entity, you can use iterators.NewSingleElement.
	//
	// Exec can execute a Query that goal is to do modification in the storage in a described way by the Query#Test method.
	//
	// By convention the Query name should start with the Task to be achieved and than followed by type.
	// 	example: UpdateUserByName, DeleteUser, InvalidateUser
	// In the case of dedicated pkg for a specific generic query use case, the pkg name can represent the CRUD functionality as well.
	//
	// You may ask why did I choose to have one entry point for each of my external resource,
	// instead having dedicated functions for each query usage required by different interactors.
	// It's because the look of the Resource was greatly affected by my own conversation,
	// which is described more deeply at the Query interface.
	// To sum it up the relevant part from the Query interface is that I choose to use raw data structures as input
	// for different use-cases with the Resource and describe the expected behavior attached to that raw data structure.
	// This way it is ensured that the specification of an expected behavior is placed next to the interactor's Query structure
	// and not defined directly into the Resource specification.
	// I personally found a great productivity and flexibility in this convention.
	Exec(Query) frameless.Iterator
}

/*

	Query is a data structure that describe expected behavior from the external resource it consume it.

		TL;DR:
			Query imposes discipline upon where you implement what, and thus separate the scope of the interactor and the external resource.
			Simply put Query is a behavior specification and a data structure that helps use-cases to focus on the "what" instead of the "how".

	The main purpose is to Use Case specific behavior requirements from the technology specific implementation
	The Use Case implementation should never specify low level implementation of the storage usage, but represent it with a Query data structure that passed to the storage.
	The Query data structure should contain all the necessary data that required for the storage.
	Query type must define a Test method that describe the expected high level behavior when storage encounter this type of query.
	This way all the storage implementations use the same shared specification/test, and by that ensure the high level requirements for the use-cases.
	The Query type should not define anything other than the data types in its structure, and the expected behavior in the Test function,
	everything else is the responsibility of the use-cases who use it.
	If possible, its data structure should only include primitive fields.
	By convention this should be declared next to the use-case implementation who use it.
	If it's a generic query structure, than it should include a field that can hold a structure that can be used as a reference for creating fixture with the fixtures pkg New function.

	By convention the Query name should start with "[EntityName][FindLogicDescription]" so it is easy to distinguish it from other exported Structures,
	example: UserByName, UsersByName, UserByEmail

	One side note here from my personal experience is that, I often saw SQL and other similar inputs was being tested,
	which I believe is a common mistake of testing implementation instead of behavior.
	By first describing the behavior before we know anything from the resource, In my opinion can help in keeping focus on the "what" more easily.

*/
type Query interface {
	// Test specify the given query - Use Case expected behavior, and must be used for implementing it in the external resource tests.
	// To cover your behavior easily it is advised to use multiple test run with different contexts.
	// I personally prefer the testing#T.Run to create test contexts.
	// test should receive a tear-down/cleanup function as the last argument that will used to reset to the initial state the external resource.
	Test(t *testing.T, r Resource)
}
