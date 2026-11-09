---
name: frameless-httpkit
description: Build HTTP APIs and API clients with go.llib.dev/frameless/pkg/httpkit. Covers Router (:name path params, Namespace, Use middleware, Mount), RESTHandler and RESTHandlerFromCRUD (REST resources served from crud repositories, nested resources with ownership scoping, auth in ResourceContext, DTO codecs, Filters), RESTClient (a crud repository backed by a remote REST API), rfc7807 problem+json errors, middleware and RoundTripper stacks, access logs and retrying transports. Use when writing or changing an HTTP handler, REST endpoint, router or server wiring; exposing a repository or use case over HTTP; mapping errors to status codes; integrating a remote REST API or replacing a hand-written HTTP client; adding auth, logging or other middleware; testing handlers with httptest or clients with crudcontract; replacing gorilla/mux, chi, gin or echo in a frameless project; or changing pkg/httpkit itself. Applies to any Go project that imports go.llib.dev/frameless.
compatibility: Go projects importing go.llib.dev/frameless/pkg/httpkit.
license: MIT (matches go.llib.dev/frameless)
metadata:
  project: go.llib.dev/frameless
  package: pkg/httpkit
  scope: Global
---

# Frameless httpkit

`go.llib.dev/frameless/pkg/httpkit` is the HTTP edge of a frameless app. It is plain `net/http`: every piece is an `http.Handler` or an `http.RoundTripper`, so it composes with the standard library, `httptest` and any third-party middleware.

| Piece | What it is |
| --- | --- |
| `Router` | a method- and path-aware `http.Handler` with `:name` placeholders, namespaces and scoped middleware |
| `RESTHandler[ENT, ID]` | a REST resource (`GET/POST/DELETE /`, `GET/PUT/DELETE /:id`) built from crud-shaped functions |
| `RESTClient[ENT, ID]` | the other side: a crud repository (`Create`, `FindAll`, `FindByID`, `Update`, `DeleteByID`, …) over a remote REST API |
| `rfc7807` | `application/problem+json` error replies, and `httpkit.ErrorMapping` for the built-in errors |
| `WithMiddleware`, `WithRoundTripper`, `AccessLog`, `RetryRoundTripper`, `Mount` | server and client plumbing |

In hexagonal terms (see `frameless-software-design`), `Router` and `RESTHandler` are **driving adapters**. They live in an adapter package such as `adapter/httpapi` and call the domain. `RESTClient` is a **driven adapter**: it can supply a domain's repository role interface and run that domain's contract (see `frameless-ports`, `frameless-crud`).

## Stop signs

If you are about to write any of these, use httpkit instead:

- a CRUD handler built from `switch r.Method` and `strings.Split(r.URL.Path, "/")` → `RESTHandler` or `RESTHandlerFromCRUD`
- adding `gorilla/mux`, `chi`, `gin` or `echo` to a project that already depends on frameless → `Router`
- `http.Error(w, err.Error(), 500)`, or an ad-hoc `{"error": "..."}` body for API errors → an `rfc7807.Handler` as the `ErrorHandler`
- a hand-written REST client: `json.NewDecoder(resp.Body)`, `if resp.StatusCode == 404`, paging through results → `RESTClient`
- a retry loop around `client.Do` → `RetryRoundTripper` (see `frameless-resilience`)
- `fmt.Sprintf("%s/users/%s", base, id)` → `pathkit.Join` with `url.PathEscape` (see `frameless-pathkit`)
- `mw1(mw2(mw3(h)))` → `httpkit.WithMiddleware(h, mw1, mw2, mw3)`
- `json:"..."` tags on domain entities to shape the wire format → a DTO mapping in the codec
- mocking `http.Client` or the handler's dependencies → `httptest`, `memory` repositories and `RoundTripperFunc`

## Router

```go
func NewHandler(c Config) http.Handler {
	return httpkit.NewRouter(func(r *httpkit.Router) {
		r.Use(httpkit.WithAccessLog) // applies to every route of this router, 404s included
		r.Get("/healthz", healthz)
		r.Namespace("/api/v1", func(r *httpkit.Router) {
			r.Use(RequireToken(c.Token)) // scoped to /api/v1/...
			r.Resource("/users", usersAPI(c))
			r.Get("/reports/:year", reports(c)) // httpkit.PathParams(r.Context())["year"]
		})
	})
}
```

| Method | Does |
| --- | --- |
| `NewRouter(func(*Router))`, `var r httpkit.Router` | create one; the zero value is ready |
| `Get`, `Post`, `Put`, `Patch`, `Delete`, `Head`, `Options`, `Connect`, `Trace`, `On(method, path, h)` | register a handler for one method |
| `Namespace(path, func(*Router))`, `Sub(path) *Router` | group routes under a prefix; `Use` inside applies to the group only |
| `Resource(path, restHandler)` | mount a `RESTHandler` at `path` (it serves `path` and `path/...`) |
| `Mount(path, h)` | a `*Router` is merged in; any other handler answers **every method** at `path` and below, and sees the path relative to `path` |
| `Handle(pattern, h)` | register on an `http.ServeMux` at this level: `/p` is exact, `/p/` a subtree; the handler sees the path relative to the namespace |
| `Use(mws...)` | add `MiddlewareFactoryFunc`s; the first one is the outermost |
| `RouteInfo()`, `httpkit.GetRouteInfo(h)` | list the routes, e.g. `fmt.Println(r.RouteInfo())` |

`httpkit.PathParams(ctx)` returns the placeholder values as `map[string]string`, already unescaped (`/a/x%2Fy` → `x/y`). `httpkit.WithPathParam(ctx, k, v)` adds one, which is mostly useful for `RESTClient`.

Matching rules, all verified:

- **A route answers its exact path only.** `r.Get("/x", h)` doesn't serve `GET /x/extra`: that is a 404, unless a mounted handler covers it.
- **A wrong method replies 405** `rest-method-not-allowed` with an `Allow` header. An unknown path replies 404 `path-not-found`. Both are rfc7807 JSON, and the middlewares of the deepest matching namespace still run.
- **Routes win over mounted handlers.** `Mount`, `Resource` and `Handle` receive a request only when no route matches it, and the deepest mount wins. So `r.Resource("/things", h)` and `r.Post("/things/:id/activate", a)` coexist. Still prefer the resource's `ResourceRoutes` for custom actions, as there they get `ResourceContext` and the entity lookup (see below).
- **Fixed segments win over placeholders.** `/a/fixed` beats `/a/:id`. Fixed segments match by their unescaped value, like `http.ServeMux` (`r.Get("/a b", h)` serves `/a%20b`).
- **Each placeholder name gets its own branch.** With `/a/:id` and `/a/:nid/x`, a request sets only the name of the route it matched. When two placeholders could match, the one registered last is tried first.
- **A mounted handler sees the path relative to its mount point.** `Mount("/m", h)` hands `/m/a/b` to `h` as `/a/b`, so don't wrap `h` in `http.StripPrefix` too. Middlewares see the original request. A `Handle` inside a `Namespace` sees the path relative to the namespace.
- **`HEAD` is not routed to `GET` routes.** Register `Head` explicitly, or the request gets a 405.

On a plain `http.ServeMux`, `httpkit.Mount(mux, "/api", h)` registers both `/api` and `/api/`, strips the prefix, and keeps the routing context, so a `RESTHandler` or `Router` below it works unchanged.

## RESTHandler

Most resources start from a repository:

```go
users := httpkit.RESTHandlerFromCRUD[User, UserID](repo, func(h *httpkit.RESTHandler[User, UserID]) {
	h.MediaType = mediatype.JSON
	h.ErrorHandler = errorHandler
})
```

`RESTHandlerFromCRUD` always sets `Show = repo.FindByID`. It sets `Create`, `Index`, `Update`, `Destroy` and `DestroyAll` when `repo` implements `crud.Creator`, `crud.AllFinder`, `crud.Updater`, `crud.ByIDDeleter` and `crud.AllDeleter`. To expose fewer operations, or to call a use case instead of the repository, set the fields yourself, either in the struct literal or in the configure function:

```go
h := httpkit.RESTHandler[Order, OrderID]{
	Create: app.PlaceOrder, // func(ctx, *Order) error
	Index:  app.ListOrders, // func(ctx) iter.Seq2[Order, error]
	Show:   app.FindOrder,  // func(ctx, OrderID) (Order, bool, error)
}
```

### Routes and status codes

| Request | Field | Success | Notes |
| --- | --- | --- | --- |
| `GET /` | `Index` | 200, list | `Filters` apply. Streamed when the codec is a `codec.StreamProducer` (JSON and ndjson are). |
| `POST /` | `Create` | 201 and the created entity | `crud.ErrAlreadyExists` → 409 |
| `DELETE /` | `DestroyAll` | 204 | see nested resources |
| `GET /:id` | `Show` | 200 and the entity | not found → 404 `entity-not-found` |
| `PUT /:id` | `Update` | 204, no body | replaces the whole entity: the body is decoded into a zero `ENT` and the ID is set from the path. `Show` runs first when set; `crud.ErrNotFound` → 404. |
| `PATCH /:id` | none | 405 | not supported: a partial update can't tell an omitted field from one set to its zero value (`0`, `false`, `""`). Send the full state with `PUT`. |
| `DELETE /:id` | `Destroy` | 204 | **needs `Show`**, `ScopeAware` or `DisableOwnershipConstraint`, otherwise 405, even at the top level. `crud.ErrNotFound` → 404. |
| `/:id/...` | `ResourceRoutes` | whatever it replies | needs `Show` (else 405); a missing entity → 404 |

- A `nil` operation replies 405 `rest-method-not-allowed`, and so does any other method on `/` or `/:id`.
- A malformed body, or a codec error, replies 400 `invalid-request-body`. An ID that `IDParser` rejects replies 400 `malformed-id-in-path`. A body over `BodyReadLimit` (default 16MB, `-1` for no limit) replies 413.
- A panic replies 500 and is logged with `logger.Error`. A `panic(http.ErrAbortHandler)` is passed on, so it still aborts the response.
- Other errors go through `ErrorHandler`: an `errorkit.UserError` becomes 400 unless mapped, anything else 500 `internal-server-error`.
- Once a streamed list has started, an `Index` or encoding error is logged and **aborts the response** with `panic(http.ErrAbortHandler)`. The client gets a broken connection or a truncated body, never a list that looks complete. An error before the first element still gets a proper error reply. In a test that calls `ServeHTTP` on a `httptest.ResponseRecorder`, the abort is a panic; use `httptest.NewServer` to see what a client sees.

### Fields

| Field | Use |
| --- | --- |
| `Create`, `Index`, `Show`, `Update`, `Destroy`, `DestroyAll` | the operations |
| `ResourceRoutes http.Handler` | routes under `/:id/`, with the prefix stripped: custom actions and sub-resources |
| `CollectionContext func(ctx) (ctx, error)` | runs before every collection request (`/`) |
| `ResourceContext func(ctx, ID) (ctx, error)` | runs before every `/:id` request, including `ResourceRoutes` |
| `ErrorHandler` | defaults to `rfc7807.Handler{Mapping: httpkit.ErrorMapping}` |
| `MediaType`, `Codecs` | the wire format; see DTOs and codecs |
| `IDParser func(string) (ID, error)`, `IDAccessor extid.Accessor` | parse the path ID, and get or set it on `ENT`; the defaults handle string and int kinds, and the `ext:"id"` tag or an `ID` field |
| `IDContextKey any` | an extra ctx key for the ID; `IDContextKey[ENT, ID]{}` is always set |
| `Filters []func(ctx, ENT) bool` | drop entities from `Index` results |
| `BodyReadLimit` | max request body size |
| `ScopeAware`, `DisableOwnershipConstraint` | see nested resources |
| `CommitManager` | WIP; don't rely on it |

### Auth, scoping and query parameters

Return an error from `CollectionContext` or `ResourceContext` to stop the request; it goes through `ErrorHandler`. `httpkit.LookupRequest(ctx)` gives you the `*http.Request` in these hooks, in the operations and in `Filters`:

```go
h.ResourceContext = func(ctx context.Context, id UserID) (context.Context, error) {
	req, _ := httpkit.LookupRequest(ctx)
	if err := auth.CanAccessUser(req, id); err != nil {
		return ctx, ErrUnauthorized
	}
	return ctxUserID.ContextWith(ctx, id), nil // contextkit.ValueHandler, read by nested handlers
}
h.Filters = []func(context.Context, Note) bool{
	func(ctx context.Context, n Note) bool {
		req, _ := httpkit.LookupRequest(ctx)
		q := req.URL.Query().Get("q")
		return q == "" || strings.Contains(n.Title, q)
	},
}
```

`Filters` run after `Index` has loaded the entities. For large collections, read the query in the `Index` closure and pass it to a repository query instead.

### Custom actions and nested resources

```go
h.ResourceRoutes = httpkit.NewRouter(func(r *httpkit.Router) {
	r.Post("/activate", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Context().Value(httpkit.IDContextKey[User, UserID]{}).(UserID)
		// ...
	}))
	r.Resource("/notes", notesAPI) // /users/:id/notes, scoped to the user
})
```

A `RESTHandler` mounted under another one's `ResourceRoutes` is **scoped to the parent entity**. The parent is loaded with `Show`, and ownership is checked with `relationship.Related(parent, child)`. Related means the child has a reference to the parent, such as `Note.UserID UserID`, a field named after the parent type when the ID is a builtin type, or a `[]NoteID` on the parent. You can also register the relationship explicitly with `relationship.BelongsTo` or `ReferencesMany`; see `port/crud/relationship/README.md`.

| Scoped request | Behaviour (verified) |
| --- | --- |
| `GET /users/1/notes` | only notes related to user 1 |
| `POST /users/1/notes` | the parent reference is set from the URL, whatever the body says |
| `GET`, `PUT`, `DELETE /users/1/notes/:id` | 404 if the note belongs to someone else |
| `PUT /users/1/notes/:id` | **the new body is ownership-checked too**: a body without the parent reference (`user_id`) gets 404. Keep the reference in the DTO. |
| `DELETE /users/1/notes` | without `ScopeAware`: `Index`, then `Destroy` one by one (405 if either is missing) |
| `PUT` with no `Show` | 405 |

- **Unrelated types fail closed.** If the child type has no reference to the parent, the list is empty and `GET /:id` is 404, while `POST` still succeeds. Set `DisableOwnershipConstraint` when the nesting is only a URL layout, and do the scoping yourself.
- **`ScopeAware: true`** promises that your operations already scope by the parent (for example, they read the user ID from ctx). Then `DestroyAll` is called directly, and `Destroy` works without `Show`. The ownership filter still runs.

### DTOs and codecs

`RESTHandler` and `RESTClient` marshal `ENT` itself with the codec for the media type. Keep the domain entity free of `json` tags and put the mapping into the codec:

```go
type NoteJSON struct {
	ID     string `json:"id,omitempty"`
	UserID string `json:"user_id,omitempty"`
	Title  string `json:"title"`
}

func NotesCodec() *jsonkit.Codec {
	var c jsonkit.Codec
	jsonkit.CodecRegister(&c, "note", jsonkit.DataTransferObjectMapping[Note, NoteJSON]{
		ToDTO: func(ctx context.Context, n Note) (NoteJSON, error) { /* ... */ },
		ToENT: func(ctx context.Context, d NoteJSON) (Note, error) { /* validation errors become 400 */ },
	})
	return &c
}

h.MediaType = mediatype.JSON
h.Codecs = httpkit.Codecs{mediatype.JSON: NotesCodec()}
```

- This covers single values, lists (streamed too), request bodies, and `RESTClient` when it is given the same codec. No `$type` is written for a top-level value or a list element.
- One codec can hold the DTO mappings of every entity of the API. `examples/restful-api/adapter/httpapi` registers `User` and `Note` on one `jsonCodec` and shares it between the resources.
- `MediaType` defaults to JSON, so a JSON codec in `Codecs` is used without it. Set `MediaType` when the API's default media type is something else.
- Codecs get no request ctx. Do request-scoped mapping, such as setting the owner from the authenticated user, in the operation closure or in `ResourceContext`.

| Default media type | Codec |
| --- | --- |
| `application/json`, `application/problem+json` | `jsonkit.Codec` |
| `application/x-ndjson`, `application/stream+json`, `application/json-stream` | `jsonkit.LinesCodec`: one JSON value per line |
| `application/x-www-form-urlencoded` | `formurlencoded.Codec`: snake_case keys or `form`/`url` tags, dotted keys for nesting, repeated keys for slices |

Request bodies are decoded by `Content-Type`, falling back to `MediaType` and then JSON. Responses are encoded by `Accept`: media ranges are ranked by `q`, `q=0` excludes a type, and `*/*` or `type/*` prefer the request's `Content-Type`, then `MediaType`, then the other codecs. When no accepted type has a codec, the reply uses the request's media type or `MediaType` instead of 406 Not Acceptable. RFC 9110 allows a server to disregard `Accept`, so clients should check the reply's `Content-Type`. Your own codec only needs `codec.Codec` (`Marshal`, `Unmarshal`); add `codec.StreamProducer` to stream lists.

## Errors: rfc7807

The default `ErrorHandler` writes `application/problem+json`:

```json
{"type":"/entity-not-found","title":"Entity not found","status":404,"detail":"The requested entity is not found in this resource."}
```

`rfc7807.Handler.ToDTO` turns an `errorkit.UserError` into `type` = `Code`, `title` = the title-cased `Code`, `detail` = `Message` and `status` 400. Anything else becomes 500 `internal-server-error`, without leaking `err.Error()`. Then `Mapping` adjusts the DTO. `httpkit.ErrorMapping` gives the built-in errors their statuses: `ErrEntityNotFound` and `ErrPathNotFound` 404, `ErrMethodNotAllowed` 405, `ErrEntityAlreadyExist` 409, `ErrRequestEntityTooLarge` 413, `ErrForbidden` 403, `ErrMalformedID` and `ErrInvalidRequestBody` 400.

**A custom `Mapping` replaces `ErrorMapping`, so call it first.** Otherwise every built-in error turns into a 400.

```go
var ErrUnauthorized = errorkit.UserError{Code: "unauthorized", Message: "Authentication is required."}

var errorHandler = rfc7807.Handler{
	BaseURL: "/errors", // type becomes "/errors/unauthorized"
	Mapping: func(ctx context.Context, err error, dto *rfc7807.DTO) {
		httpkit.ErrorMapping(ctx, err, dto)
		switch {
		case errors.Is(err, ErrUnauthorized):
			dto.Status = http.StatusUnauthorized
		case errors.Is(err, domain.ErrInsufficientFunds): // a domain error that isn't a UserError
			dto.Type.ID, dto.Title, dto.Status = "insufficient-funds", "Insufficient funds", http.StatusUnprocessableEntity
		}
		dto.Extensions = LegacyEnvelope{Error: LegacyError{Code: dto.Type.ID, Message: dto.Detail}} // flattened into the JSON
	},
}
```

- Prefer `errorkit.UserError` values for errors that callers can act on: they carry a stable code and a safe message.
- Use the same handler in plain handlers and middleware: `errorHandler.HandleError(w, r, err)`.
- When both the error and the request ctx are `context.Canceled`, it writes nothing.
- Clients decode the body into `rfc7807.DTO`, or into `rfc7807.DTOExt[Ext]` for typed extensions.

## RESTClient

```go
client := httpkit.RESTClient[Note, NoteID]{
	BaseURL: "https://notes.example.com/api/v1/users/:user_id/notes",
	HTTPClient: &http.Client{
		Timeout:   25 * time.Second,
		Transport: httpkit.WithRoundTripper(httpkit.RetryRoundTripper{}, WithBearer(token)),
	},
	MediaType: mediatype.JSON,
	Codecs:    httpkit.Codecs{mediatype.JSON: httpapi.NotesCodec()},
}

ctx = httpkit.WithPathParam(ctx, "user_id", string(userID)) // the raw value; the client escapes it
note, found, err := client.FindByID(ctx, noteID)
```

| Method | Request | Result mapping |
| --- | --- | --- |
| `Create(ctx, *ENT)` | `POST base` | the response is decoded into `*ENT`; 409 → `crud.ErrAlreadyExists` |
| `FindAll(ctx)` | `GET base` | `iter.Seq2[ENT, error]`, streamed unless `DisableStreaming`; 404 → `crud.ErrNotFound` |
| `FindByID(ctx, id)` | `GET base/id` | 404 → `found == false` |
| `FindByIDs(ctx, ids...)` | `FindByID` per ID, prefetching up to `PrefetchLimit` (default 20, `-1` off) | a missing ID → `crud.ErrNotFound` |
| `Update(ctx, *ENT)` | `PUT base/id`, then `FindByID` to refresh `*ENT` | 404 → `crud.ErrNotFound`; needs the ID accessor (`IDA`, default `extid`) |
| `DeleteByID(ctx, id)` | `DELETE base/id` | 404 → `crud.ErrNotFound` |
| `DeleteAll(ctx)` | `DELETE base` | 404 → `crud.ErrNotFound` |

- Any other non-2xx status returns `httpkit.ClientErrUnexpectedResponse{StatusCode, Body, URL, RequestMethod, RequestURL}`. Use `errors.As`, then decode `Body` into `rfc7807.DTO` if the server speaks rfc7807.
- **It path-escapes the formatted ID and the `WithPathParam` values**, so an ID with `/`, `%` or `?` reaches the right URL. Give `IDFormatter` and `WithPathParam` raw values: escaping them yourself double-escapes (`a/b` becomes `a%252Fb`).
- `:name` placeholders in `BaseURL` come from `WithPathParam` on the call's ctx, or from the `WithContext` hook for a client that is always scoped. A missing value returns a `missing path param: name` error.
- `HTTPClient` defaults to `httpkit.DefaultRestClientHTTPClient`: a 25s timeout and `RetryRoundTripper` with `ExponentialBackoff{Delay: time.Second, Timeout: time.Minute}`. If you set your own client, add `RetryRoundTripper` yourself (see `frameless-resilience`).
- Response bodies are limited by `BodyReadLimit` (`ErrResponseEntityTooLarge`). A response with an unknown `Content-Type` is an error.
- For a third-party API that doesn't follow these REST conventions, write a small adapter with `http.Client`, `pathkit.Join` and `RetryRoundTripper` instead of forcing `RESTClient` onto it.

Because `RESTClient` is a crud repository, it can supply a domain's repository role interface directly (`var _ notes.NoteRepository = httpkit.RESTClient[notes.Note, notes.NoteID]{}`). Run the domain's contract, or `crudcontract`, against it.

## Middleware and transports

```go
func RequireToken(token string) httpkit.MiddlewareFactoryFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				errorHandler.HandleError(w, r, ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func WithBearer(token string) httpkit.RoundTripperFactoryFunc {
	return func(next http.RoundTripper) http.RoundTripper {
		return httpkit.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			r = r.Clone(r.Context()) // a RoundTripper must not modify the caller's request
			r.Header.Set("Authorization", "Bearer "+token)
			return next.RoundTrip(r)
		})
	}
}
```

| Helper | Notes |
| --- | --- |
| `WithMiddleware(h, mws...)` | the first middleware is the outermost; `nil` entries are skipped |
| `WithRoundTripper(transport, rts...)` | the first factory is the outermost; a `nil` transport means `http.DefaultTransport` |
| `WithAccessLog`, `AccessLog{Next, AdditionalLoggingDetail}` | logs `http-access-log` through `pkg/logger` with the method, path, query, duration, status, remote address and body lengths |
| `RetryRoundTripper` | an alias of `resilience.HTTPRoundTripper` |
| `RoundTripperFunc` | a function as an `http.RoundTripper`; handy in tests |
| `NewClient()` | an `*http.Client` with a 30s timeout, and no retries |
| `IsSuccess(code or *http.Response)` | true for 2xx |

## Testing

Test HTTP adapters in-process with `httptest`, real handlers and `memory` repositories. Don't mock. Follow `testcase-spec`. The full, verified example for a nested API is in [references/worked-example.md](references/worked-example.md); this sketch is for a flat `/notes` resource:

```go
func TestNewHandler(t *testing.T) {
	s := testcase.NewSpec(t)
	logger.Testing(t) // captures the access logs and the handler's logs in the test output

	var (
		repo = let.Var(s, func(t *testcase.T) *memory.Repository[Note, NoteID] {
			return memory.NewRepository[Note, NoteID](memory.NewMemory())
		})
		subject = let.Var(s, func(t *testcase.T) http.Handler {
			return httpapi.NewHandler(httpapi.Config{Notes: repo.Get(t)})
		})
	)

	s.Describe("GET /notes/:id", func(s *testcase.Spec) {
		var (
			note = let.Var(s, func(t *testcase.T) Note {
				n := Note{Title: t.Random.String()}
				assert.Must(t).NoError(repo.Get(t).Create(context.Background(), &n))
				return n
			})
			id = let.Var(s, func(t *testcase.T) NoteID { return note.Get(t).ID })
		)
		act := func(t *testcase.T) *httptest.ResponseRecorder {
			rr := httptest.NewRecorder()
			subject.Get(t).ServeHTTP(rr, httptest.NewRequest(http.MethodGet,
				pathkit.Join("/notes", url.PathEscape(string(id.Get(t)))), nil))
			return rr
		}

		s.Then("it replies the note", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, http.StatusOK, rr.Code)
			var got httpapi.NoteJSON
			assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			assert.Equal(t, string(note.Get(t).ID), got.ID)
		})

		s.When("the note doesn't exist", func(s *testcase.Spec) {
			id.Let(s, func(t *testcase.T) NoteID { return NoteID(t.Random.UUID()) })

			s.Then("it replies 404 entity-not-found", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, http.StatusNotFound, rr.Code)
				var dto rfc7807.DTO
				assert.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dto))
				assert.Equal(t, httpkit.ErrEntityNotFound.Code.String(), dto.Type.ID)
			})
		})
	})
}
```

- In handler tests, build request paths with `pathkit.Join` and `url.PathEscape`, as `httptest.NewRequest` takes the raw URL. Include IDs containing `/` or `%` to prove that escaping round-trips.
- Assert error replies by `dto.Type.ID` against `httpkit.ErrX.Code.String()` or your own `UserError` codes, not by message text.
- **Clients:** run `crudcontract` (`Creator`, `Finder`, `ByIDsFinder`, `Updater`, `Deleter`) against a `RESTClient`. Point it at `httptest.NewServer(httpkit.RESTHandlerFromCRUD(memoryRepo))` with `HTTPClient: srv.Client()`, or at your real handler. For scoped clients, set `crudcontract.Config.MakeContext` or the client's `WithContext` with `WithPathParam`. See `frameless-crud`.
- **No server needed:** `HTTPClient: &http.Client{Transport: httpkit.RoundTripperFunc(...)}` that calls `h.ServeHTTP` on a `httptest.ResponseRecorder` and returns `rr.Result()`. Return `r.Context().Err()` first, because the recorder ignores cancellation and the contract checks it.
- **Middleware:** `tchttp.HandlerMiddleware(func(t *testcase.T, next http.Handler) http.Handler { ... }).Spec(s)` and `tchttp.RoundTripperMiddleware(...)` (from `go.llib.dev/testcase/tchttp`) run a ready-made spec against your middleware. See `pkg/httpkit/router_test.go` and `httpkit_test.go`.
- Test against third-party APIs with a fake server (`httptest.NewServer`) that implements the subset you use, behind the same contract as the real adapter (see `frameless-ports`).

## Changing `pkg/httpkit` itself

- Keep it `net/http`-native: handlers stay `http.Handler`, clients take `*http.Client`, and the routing state travels in the request ctx (`internal/context.go`, `internal/routes.go`).
- Build paths with `pkg/pathkit`, and retries with `pkg/resilience`.
- Write specs in `testcase` style next to the existing ones. Several `// TODO:TEST_ME` branches in `resthandler.go` have no test yet.
- Run:

  ```sh
  go test ./pkg/httpkit/... ./pkg/pathkit/... ./pkg/resilience/... ./pkg/devops/health/...
  (cd examples/restful-api && go vet ./... && go test ./...)
  ```

  `examples/restful-api` uses `replace go.llib.dev/frameless => ../..`, so it builds against your changes. `adapter/hashicorp/vault` is a separate module pinned to a released frameless version; it only picks up changes when that version is bumped.
- Update `pkg/httpkit/README.md`, and this skill, when behaviour changes.
- Known quirks. Don't build on them, and pin any fix with a test:
  - `Router`: `Mount` of a `*Router` merges its routes, but drops a handler mounted at the merged router's root, and a fixed path segment that both routers have is replaced by the merged router's subtree rather than merged. `HEAD` isn't routed to `GET` routes.
  - `dto.go` is commented-out WIP, and `CommitManager` is WIP.

## Reference

- `pkg/httpkit/router.go`: `Router`, `PathParams`, `RouteInfo`
- `pkg/httpkit/resthandler.go`: `RESTHandler`, `RESTHandlerFromCRUD`, `RESTOwnershipCheck`, `IDContextKey`
- `pkg/httpkit/restclient.go`: `RESTClient`, `DefaultRestClientHTTPClient`, `ClientErrUnexpectedResponse`
- `pkg/httpkit/httpkit.go`: `Mount`, `WithMiddleware`, `WithRoundTripper`, `AccessLog`, `RoundTripperFunc`, `LookupRequest`
- `pkg/httpkit/errors.go`: the built-in `UserError`s and `ErrorMapping`
- `pkg/httpkit/codec.go`, `params.go`, `mime.go`, `internal/`: codecs, ID conversion, path params, routing context
- `pkg/httpkit/rfc7807/`: `Handler`, `DTO`, `DTOExt`; `example_test.go` shows a company error envelope
- `pkg/httpkit/mediatype/`, `pkg/httpkit/formurlencoded/`
- `pkg/httpkit/*_test.go`: `resthandler_test.go`, `restclient_test.go` (crudcontract wiring), `router_test.go` (tchttp)
- `pkg/jsonkit/codec.go`: `CodecRegister`, `DataTransferObjectMapping`
- `port/crud/relationship/README.md`: how parent and child are related
- `examples/restful-api/`: a small app with nested resources, auth in `ResourceContext`, and an API-wide JSON codec with DTO mappings
- [references/worked-example.md](references/worked-example.md): a complete nested API, its spec, and a contract-tested client

## Checklist

- [ ] HTTP code lives in an adapter package and calls the domain; no `net/http` types in the domain.
- [ ] Routes are built with `Router`, and REST resources with `RESTHandler` or `RESTHandlerFromCRUD`, not hand-written method and path switches.
- [ ] Custom actions and sub-resources are in `ResourceRoutes`, not registered under the resource path on the parent router.
- [ ] `Destroy` comes with `Show`, or the handler is `ScopeAware` or `DisableOwnershipConstraint` on purpose.
- [ ] Nested child types reference their parent, or `DisableOwnershipConstraint` is set deliberately. DTOs keep the parent reference so scoped `PUT`s pass.
- [ ] API docs and clients treat `PUT` as a full replacement; a `RESTHandler` has no `PATCH` (it replies 405).
- [ ] Domain entities have no wire tags; the DTO mapping is registered on the codec (`jsonkit.CodecRegister` with `DataTransferObjectMapping`).
- [ ] Errors reply rfc7807. A custom `Mapping` calls `httpkit.ErrorMapping` first, and caller-facing errors are `errorkit.UserError`s.
- [ ] Auth and scoping happen in middleware, `CollectionContext` or `ResourceContext`, and use `LookupRequest` for the request.
- [ ] `RESTClient` gets raw IDs and path params (it escapes them), keeps retries, and passes `crudcontract` or the domain contract.
- [ ] Tests use `httptest`, `memory` repositories and `logger.Testing(t)`, assert error `Type.ID`s, and use IDs that need escaping.
