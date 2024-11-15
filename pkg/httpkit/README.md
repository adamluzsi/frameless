# httpkit

This package provides a set of utilities for working with HTTP requests and responses.

## RESTful API tools

### What is REST?

REST, short for Representational State Transfer,
is an architectural style for designing networked applications.
It was introduced by Roy Fielding in his 2000 PhD dissertation.

The primary goals of a RESTful API are to:

- Provide a uniform interface for interacting with resources (CRUD over HTTP)
- Separate concerns between client and server
- Use standard HTTP methods (e.g., GET, POST, PUT, DELETE) to manipulate resources

### RESTHandler

RESTHandler implements an http.Handler that adheres to the Representational State of Resource (REST) architectural style.

```go
func example() {
	fooRepository := memory.NewRepository[X, XID](memory.NewMemory())
	fooRestfulResource := httpkit.RESTHandler[X, XID]{
		Create:  fooRepository.Create,
		Index:   fooRepository.FindAll,
		Show:    fooRepository.FindByID,
		Update:  fooRepository.Update,
		Destroy: fooRepository.DeleteByID,

		Mapping: dtokit.Mapping[X, XDTO]{}, 

		MediaType: mediatype.JSON, // we can set the preferred default media type in case the requester don't specify it.

		MediaTypeMappings: httpkit.MediaTypeMappings[X]{ // we can populate this with any media type we want
			mediatype.JSON: dtokit.Mapping[X, XJSONDTO]{},
		},

		MediaTypeCodecs: httpkit.MediaTypeCodecs{ // we can populate with any custom codec for any custom media type
			mediatype.JSON: jsonkit.Codec{},
		},
	}

	mux := http.NewServeMux()
	httpkit.Mount(mux, "/foos", fooRestfulResource)
}
```

#### Automatic Resource Relationships

One of the key features of our RESTHandler is its ability to automatically infer relationships between resources.
This means that when you define a nested URL structure, such as `/users/:user_id/notes/:note_id/attachments/:attachment_id`,
our handler will automatically associate the corresponding entities and persist their relationships.

For example, if we have three entities: User, Note, and Attachment, where:

* A Note belongs to a User (identified by `Note#UserID`)
* A Note has many Attachments (identified by `Note#Attachments`)

When someone creates a new Note, our handler will automatically infer the UserID from the URL parameter `:user_id`.
Similarly, when accessing the path `/users/:user_id/notes`, our handler will return only the notes that are scoped to the specified user.

#### Ownership Constraints

Our RESTHandler provides a solid foundation for building RESTful APIs that meet the primary goals of REST.
With its automatic resource relationship inference and ownership constraint features,
you can focus on building robust and scalable applications with ease.

In RESTful APIs, relationships between entities are typically represented using nested paths for 1:N relationships.
For instance, if a User has multiple Notes, the API endpoint would be structured as `/users/:user_id/notes/:note_id`.

With the new REST Scoping logic in RESTHandler, controllers gain the automatic ability to establish associations between entities.
This approach leverages entity relationships to ensure consistency. 
For example, the UserID field in a Note can be automatically set to match the :user_id value specified in the path.

When a RESTHandler acts as a subresource but is not configured as ScopeAware, it restricts the results to entities related to the parent entity in the scope. For instance, accessing /users/42/notes will return only the Notes associated with the User whose ID is 42.

This enhancement ensures that entity relationships are enforced directly within the API structure, promoting both clarity and reliability.

If your RESTHandler operations are scope aware by default, you can disable this behavior by setting `ScopeAware: true` in the RESTHandler.

## RoundTripperFunc

RoundTripperFunc is a type that allows you to create an HTTP RoundTripper
from a function that takes an HTTP request and returns an HTTP response and an error.
This is useful for creating custom middleware to be used with the http.Client or the http.Transport.

## RetryRoundTripper

RetryRoundTripper is a type that wraps an existing http.RoundTripper
and provides a mechanism for retrying failed requests.
This is useful for dealing with transient errors that can occur when making HTTP requests.

The RetryRoundTripper retries requests in case of a recoverable error
(such as network timeouts or temporary server errors) up to a certain number of times.
If the retries exceed the maximum number of retries, the last response and error are returned.

The RetryRoundTripper considers the following errors as retriable:

- http.ErrHandlerTimeout
- net.ErrClosed,
- and timeout errors

The RetryRoundTripper considers HTTP responses with the following status codes as temporary errors:

- Internal server error
- Bad gateway
- Gateway timeout
- Service unavailable  
- Insufficient storage
- Too many requests
- Request timeout
