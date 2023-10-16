# CRUD port

Using formalized CRUD (Create, Read, Update, Delete) interfaces in your software design has numerous benefits
that can lead to more robust, maintainable, and scalable applications.
It also reduce the learning curve for new team members.

## Easy Testing

When you create an adapter for a specific role interface, 
you can effortlessly incorporate the CRUD contract testing suite. 
This suite verifies that your implementation behaves consistently with other solutions, 
meeting the expectations of the domain code.

```go
package adapter_test

import (
	"context"
	"testing"
	
	"github.com/adamluzsi/testcase/assert"

	"go.llib.dev/frameless/ports/crud/crudcontracts"
)

func TestMyAdapter(t *testing.T) {
	crudcontracts.SuiteFor[
		mydomain.Foo, mydomain.FooID,
		myadapter.FooRepository,
	](func(tb testing.TB) crudcontracts.SuiteSubject[
		mydomain.Foo, mydomain.FooID,
		myadapter.FooRepository,
	] {
		c, err := myadapter.NewConnection(...)
		assert.NoError(tb, err)
		tb.Cleanup(func() { c.Close() })
		r := myadapter.NewFooRepository(c)
		return crudcontracts.SuiteSubject[
			mydomain.Foo, mydomain.FooID,
			myadapter.FooRepository,
		]{
			Resource:              r,
			CommitManager:         c,
			MakeContext:           context.Background,
			MakeEntity:            func() mydomain.Foo { return /* make random Entity here without ID */ },
			CreateSupportIDReuse:  true,
			CreateSupportRecreate: true,
		}
	}).Test(t)
}
```

## Notable benefits of using the `crud` port

### Consistency

CRUD interfaces provide a standardized way to interact with your application's data, 
ensuring that developers can easily understand and work with the codebase. 
This promotes a consistent design pattern across the entire application, making it easier to maintain and extend.

### Abstraction

CRUD interfaces abstract the underlying implementation details of data storage and manipulation, 
allowing developers to focus on the business logic instead of the specifics of the data source. 
This means that you can easily swap out the data source (e.g., from a local file to a remote database) 
without having to change the application code.

### Flexibility 

By using CRUD interfaces, you can easily represent a RESTful API resource from an external system 
or expose your own entities on your HTTP API.

### Caching

Wrapping a repository that uses CRUD interfaces with caching is straightforward 
and can be done without leaking implementation details into your domain layer. 
This can improve performance by reducing the number of calls to the underlying data source, 
which might be slow or have limited resources.

## Example use-cases where utilising CRUD port can benefit your system design

TL;DR: using formalized CRUD interfaces in your software design offers consistency, abstraction, flexibility,
and the ability to easily incorporate caching.
By employing these interfaces, you can more easily interact with external RESTful API resources,
expose your entities on your HTTP API, and improve your application's performance. 
The provided Go code demonstrates a set of CRUD interfaces that can be implemented to achieve these benefits.

### Repository Pattern

The Repository pattern is a design pattern used in software development 
to abstract the way data is stored, fetched, and manipulated. 
It acts as a middle layer between the data source (such as a database or an API) 
and the business logic of the application. By decoupling the data access logic from the rest of the application,
the Repository pattern promotes separation of concerns, maintainability, and testability.

In the Repository pattern, a repository is responsible for performing CRUD (Create, Read, Update, Delete) operations
on a specific entity or a group of related entities. 
It provides a consistent interface to interact with the underlying data source, 
allowing developers to focus on the business logic rather than the specifics of data access. 
This also makes it easier to switch to a different data source 
or introduce new data sources without having to modify the application's core logic.

### Working with External System's RESTful API as Resource

Imagine you want to integrate an external system, like a third-party API, into your application.
By using CRUD interfaces, you can define a repository that communicates with the external API
and maps the external resources to your internal data models.
The CRUD operations can then be used to interact with the external system in a standardized way.

### Exposing Entities on HTTP API

Suppose you want to expose your application's entities on an HTTP API.
Using CRUD interfaces, you can create a generic RESTful handler that uses your repository as a data source.
This handler can then be used to handle requests and perform the necessary CRUD operations on your entities,
making it easy to implement the HTTP API without having to write custom code for each operation.
