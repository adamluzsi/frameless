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

	"github.com/adamluzsi/frameless/ports/crud/crudcontracts"
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

## Notable benefits

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

### RESTful resources 

The RESTful API design is based on CRUD (Create, Read, Update, Delete) interactions 
because they represent the fundamental operations required to manage resources in a system. 
REST (Representational State Transfer) is an architectural style for designing networked applications
that emphasizes the use of standardized methods to interact with resources, making it a natural fit for CRUD operations. 
By basing RESTful APIs on CRUD, developers can create consistent, easily understood interfaces for managing resources,
leading to more maintainable and scalable applications.

Here's a brief summary of how CRUD operations relate to RESTful API methods:

CREATE (CRUD) <-> POST (REST): In CRUD, the CREATE operation is used to add a new resource to the system.
In REST, the POST method is used to create a new resource,
typically by sending the resource data to the server in the request body. 
The server processes the data and creates a new resource, returning its identifier (e.g., URL) in the response.

Example: POST /resources 
READ (CRUD) <-> GET (REST): The READ operation in CRUD corresponds to retrieving a resource 
or a collection of resources from the system. In REST, the GET method is used to request resource data,
either for a single resource (by ID) or a collection of resources.

Examples:

GET /resources (List all resources)
GET /resources/:id (Show a specific resource by ID)
UPDATE (CRUD) <-> PUT or PATCH (REST): In CRUD, 
the UPDATE operation is used to modify an existing resource in the system. 
In REST, the PUT method is used to update an entire resource by replacing its current representation with a new one, 
while the PATCH method is used to partially update a resource, modifying only specific fields.

Examples:

PUT /resources/:id (Update a resource entirely)
PATCH /resources/:id (Partially update a resource)
DELETE (CRUD) <-> DELETE (REST): The DELETE operation in CRUD corresponds to removing a resource from the system. 
In REST, the DELETE method is used to request the deletion of a resource, typically identified by its URL or ID.

Example: DELETE /resources/:id

In summary, RESTful API design is based on CRUD interactions because they represent the essential operations needed
to manage resources. By aligning REST methods with CRUD operations, developers can create consistent, 
intuitive interfaces that are easy to understand and maintain.

#### External RESTful API Resource

Imagine you want to integrate an external system, like a third-party API, into your application.
By using CRUD interfaces, you can define a repository that communicates with the external API 
and maps the external resources to your internal data models. 
The CRUD operations can then be used to interact with the external system in a standardized way.

#### Exposing Entities on HTTP API

Suppose you want to expose your application's entities on an HTTP API. 
Using CRUD interfaces, you can create a generic RESTful handler that uses your repository as a data source. 
This handler can then be used to handle requests and perform the necessary CRUD operations on your entities,
making it easy to implement the HTTP API without having to write custom code for each operation.
