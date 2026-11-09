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
		Create:     fooRepository.Create,
		Index:      fooRepository.FindAll,
		Show:       fooRepository.FindByID,
		Update:     fooRepository.Update,
		Destroy:    fooRepository.DeleteByID,
		DestroyAll: fooRepository.DeleteAll,

		MediaType: mediatype.JSON, // we can set the preferred default media type in case the requester don't specify it.

		Codecs: httpkit.Codecs{ // we can populate with any custom codec for any custom media type
			mediatype.JSON: &jsonkit.Codec{},
		},
	}

	mux := http.NewServeMux()
	httpkit.Mount(mux, "/foos", fooRestfulResource)
}
```

The codec of a media type defines the wire format of the entity.
If the API should expose a DTO rather than the entity itself,
put the mapping into the codec of that media type.
With JSON, register the mapping on a `jsonkit.Codec`,
which then writes and reads the entity through its DTO, in single values, lists and request bodies alike:

```go
func FooJSONCodec() *jsonkit.Codec {
	var c jsonkit.Codec
	jsonkit.CodecRegister(&c, "foo", jsonkit.DataTransferObjectMapping[Foo, FooJSONDTO]{
		ToDTO: func(ctx context.Context, ent Foo) (FooJSONDTO, error) { /* ... */ },
		ToENT: func(ctx context.Context, dto FooJSONDTO) (Foo, error) { /* ... */ },
	})
	return &c
}
```

`examples/restful-api/adapter/httpapi` registers the DTOs of an API's entities on one such codec.

The response media type is negotiated with the `Accept` header:
media ranges are ranked by their `q` weight, `q=0` excludes a media type, and `*/*` or `type/*` wildcards are supported.
When none of the accepted media types has a codec,
the handler replies with the media type of the request, or with `MediaType` (JSON by default), rather than with 406 Not Acceptable,
as RFC 9110 (§12.5.1) allows a server to disregard the `Accept` header.
The `Content-Type` of the reply tells the client which media type it got.

`PUT /:id` replaces the entity: the request body carries its complete new state.
`PATCH` is not supported, and replies with 405 Method Not Allowed.
A partial update can't tell a field the request body omits from one set to its zero value (`0`, `false`, `""`),
so it would either ignore intended changes or wipe fields the client didn't mention.

When `Index` fails after the list's streaming already started,
the handler aborts the response (`http.ErrAbortHandler`), so the client can't mistake a truncated list for a complete one.

A custom `ErrorHandler` should call `httpkit.ErrorMapping` in its `rfc7807.Handler.Mapping`,
to keep the status codes of httpkit's own errors, like 404 for `httpkit.ErrEntityNotFound`.

### Router

`Router` is an `http.Handler` that routes requests by their path.

- `Get`, `Post`, `Put`, `Patch`, `Delete`, etc. (`On`) register a route that only matches its exact path.
  When the path has routes, but not for the request's method, the reply is 405 Method Not Allowed, with an `Allow` header.
- `Mount`, `Resource` and `Handle` register a handler for a path and every path under it.
  When no route matches a request, the handler mounted the closest to the request path receives it.
  A mounted handler receives the request path relative to its mount point: for `Mount("/foo", h)`, `/foo/bar` reaches `h` as `/bar`.
- Path segments like `:id` are placeholders, which values are available with `httpkit.PathParams`.
  Literal path segments are matched by their unescaped value, like with `http.ServeMux`.

```go
func example() {
	var router httpkit.Router
	router.Resource("/foos", fooRestfulResource)
	router.Post("/foos/:id/activate", activateHandler)
	router.Mount("/static", http.FileServer(http.Dir("./public")))
}
```

### RESTClient

`RESTClient` is a `crud` repository that consumes a RESTful resource, like the one a `RESTHandler` serves.
It path escapes the formatted IDs and the path parameters it substitutes into its `BaseURL` (`/foos/:foo_id/bars`),
so `IDFormatter` and `httpkit.WithPathParam` should receive the unescaped values.

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
The same scope applies to `DELETE /users/42/notes`: it removes only the Notes of User 42, one by one with `Destroy`, and never calls the unscoped `DestroyAll`.

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
