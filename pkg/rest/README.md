# package `rest`

REST API stands for Representational State Transfer
and is an architectural pattern for creating web services.

Roy Fielding developed it in 2000
and has led to a growing collection of RESTful web services
that follow the REST principles.
Now, REST APIs see widespread use by application developers
due to how simply it communicates with other machines
over complex operations like COBRA, RPC, or Simple Object Access Protocol (SOAP).

REST is a ruleset that defines best practices for sharing data between clients and the server.
It’s essentially a design style used when creating HTTP
or other APIs that only asks you to use CRUD functions,
regardless of the complexity.

REST applications use HTTP methods like `GET`, `POST`, `DELETE`, and `PUT`.
REST emphasizes the scalability of components and the simplicity of interfaces.

While neglecting a portion of your tools may seem counterintuitive,
it ultimately forces you to describe complex behaviours in simple terms.

## Not all HTTP APIs are REST APIs.

The API needs to meet the following architectural requirements listed below to be considered a REST API.

These constraints combine to create an application with strong boundaries and a clear separation of concerns.
The client receives server data when requested.
The client manipulates or displays the data.
The client notifies the server of any state changes.
REST APIs don’t conceal data from the client, only implementations.

## Client-server

REST applications have a server that manages application data and state.
The server communicates with a client that handles the user interactions.
A clear separation of concerns divides the two components.

## Stateless

Servers don’t maintain client state; clients manage their application state.
The client’s requests to the server contain all the information required to process them.

## Cacheable

Being cacheable is one of the architectural constraints of REST.
The servers must mark their responses as cacheable or not.
Systems and clients can cache responses when convenient to improve performance.
They also dispose of non-cacheable information, so no client uses stale data.

## per HTTP methods

`GET` requests should be cachable by default – until an exceptional condition arises.
Usually, browsers treat all GET requests as cacheable.

`POST` requests are not cacheable by default but can be made cacheable if
either an Expires header or a Cache-Control header with a directive,
to explicitly allows caching to be added to the response.

Responses to `PUT` and `DELETE` requests are not cacheable.
Please note that HTTP dates are always expressed in GMT, never local time.

## Cache-Control Headers

Below given are the main HTTP response headers that we can use to control caching behaviour:

### Expires Header

The Expires HTTP header specifies an absolute expiry time for a cached representation.
Beyond that time, a cached representation is considered stale and must be re-validated with the origin server.
The server can include time up to one year in the future to indicate a never expiring cached representation.

```
Expires: Fri, 20 May 2016 19:20:49 GMT
```

### Cache-Control Header

The header value comprises one or more comma-separated directives.
These directives determine whether a response is cacheable
and, if so, by whom and for how long, e.g. max-age directives.

```
Cache-Control: max-age=3600
```

Cacheable responses (whether to a GET or a POST request) should also include a validator — either an ETag or a Last-Modified header.

### ETag Header

An ETag value is an opaque string token that a server associates with a resource to identify the state of the resource over its lifetime uniquely.
If the resource at a given URL changes, the server must generate a new Etag value.
A comparison of them can determine whether two representations of a resource are the same.

While requesting a resource, the client sends the ETag in the `If-None-Match` header field to the server.
The server matches the Etag of the requested resource and the value sent in the If-None-Match header.
If both values match, the server sends back a 304 Not Modified status without a body,
which tells the client that the cached response version is still good to use (fresh).

```
ETag: "abcd1234567n34jv"
```

### Last-Modified Header

Whereas a response’s Date header indicates when the server generated the response,
the Last-Modified header indicates when the server last changed the associated resource.

This header is a validator to determine if the resource is the same as the previously stored one by the client’s cache.
Less accurate than an ETag header, it is a fallback mechanism.

The Last-Modified value cannot be greater than the Date value.
Note that the Date header is listed in the forbidden header names.

## Uniform interface

Uniform interface is REST’s most well-known feature or rule. Fielding says:

> The central feature that distinguishes the REST architectural style
from other network-based styles is its emphasis on a uniform interface between components.

REST services provide data as resources with a consistent namespace.

## Layered system

Components in the system cannot see beyond their layer.
This confined scope allows you to add load-balancers easily
and proxies to improve authentication security or performance.
