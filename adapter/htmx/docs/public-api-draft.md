# Draft: public API for `adapter/htmx`

**Status:** design proposal, not implemented. Signatures and examples below describe the proposed API, not what callers can import today. This draft changes no Go code.

**Package:** `go.llib.dev/frameless/adapter/htmx`

The recommendation is **a `net/http` adapter for resource-backed HTML, with first-class `html/template` support**. An `App` registers named resources and pages, binds them to a base path when mounted, and makes resource endpoints available to templates. Resource operations follow the `httpkit.RESTHandler` idiom: wire CRUD ports directly, or supply application-specific functions for scoping, filtering and other logic.

**Revision focus:** the resource/App design in §§2 and 11 is deliberately high-level. Names and call shapes illustrate the intended experience; registration interfaces, rendering callbacks and mount-time internals are not settled API commitments. This supersedes the earlier assets-only `App` proposal. The lower-level rendering and protocol sections remain supporting proposals.

The starting point is the [research summary](research/README.md), especially the [stdlib template findings](research/stdlib-html-template.md), [helper-library comparison](research/go-htmx-helper-libraries.md) and [protocol comparison](research/htmx-4-server-contract.md). Where this draft narrows or corrects a research proposal, the decision is called out below.

## 1. Design decisions

| Decision                                 | Proposed contract                                                                                                                      |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Keep HTML templates                      | `html/template`, `fs.FS`, named templates and ordinary Go view models. No new template language.                                       |
| Keep HTTP handlers                       | Everything works with `http.Handler`, `httpkit.Router` and `http.ServeMux`. No mandatory middleware or global registry.                |
| Separate rendering from HTTP             | A `Component` writes HTML; `Render` handles representation selection, buffering, headers and status.                                   |
| Make the whole-page alternative explicit | A `View` contains a full-page component and a partial component. Never silently insert a whole document into a fragment target.        |
| Support other renderers structurally     | templ already satisfies `Component`; gomponents needs a small adapter. Neither is a dependency.                                        |
| Treat client headers as hints            | htmx 2/4 detection is heuristic, not authentication or protocol negotiation. Unknown request types conservatively request a full page. |
| Keep response state explicit             | Return a `Response` from a handler. Do not accumulate mutable response instructions in `context.Context`.                              |
| Register resources through `App`          | `App` binds named resource handlers, pages, template endpoint helpers and browser assets to one mount. Business logic and server lifecycle stay outside it. |
| Follow the RESTHandler idiom              | CRUD-shaped function fields support direct repository wiring and application-specific operations without changing the templates. |
| Resolve URLs, not data, in templates      | `endpoint` returns a mount-aware resource URL. The later HTTP request performs authentication, querying and rendering. |
| Prefer a smaller first release           | Resource registration is in scope. Defer arbitrary multi-target translation, SSE, a form framework, automatic admin UI generation and template linting. |

### Proposed surface at a glance

| Area                 | Public API                                                                      |
| -------------------- | ------------------------------------------------------------------------------- |
| Incoming requests    | `Request`, `RequestType`, `Element`, `Version`, `ParseRequest`                  |
| Renderable HTML      | `Component`, `ComponentFunc`, `Template`, `Node`                                |
| Stdlib template sets | `TemplateConfig`, `Templates`, `ParseTemplates`, `TemplateSample`, `FuncMap`    |
| HTTP responses       | `View`, `Response`, `ResponseHeaders`, `Event`, `Swap`, `Render`, `RenderError` |
| Handler adapter      | `Handler`                                                                       |
| Navigation           | `Redirect`, `Location` returning response values                                |
| HTML errors          | `ErrorHandler`, implementing `httpkit.ErrorHandler`                             |
| Resource adapters    | `ResourceHandler[ENT, ID]`, `ResourceHandlerFromCRUD`, declarative resource views (working names) |
| Registration/mounting | `App.Resource`, `App.Page`, `App.Mount`; mount-bound `endpoint` and `htmxScript` helpers (conceptual) |
| Test support         | `htmxtest.WithRequest`; use existing `httptest` and testcase assertions         |

All structs with exported maps, slices or pointers are **configuration/request-owned values**, not deeply immutable values. Configure shared objects before serving, and do not mutate referenced data concurrently with rendering.

## 2. What normal usage should look like

The intended flow is **configure → register → mount → render a page → fetch resource HTML**. The registration example below is a conceptual sketch, not a complete application or a final set of signatures. `repo`, `templateFS`, `User` and `UserID` belong to the application.

```go
app := &htmx.App{
    Templates: htmx.TemplateConfig{
        FS:     templateFS,
        Shared: []string{"templates/layout.go.html"},
        Pages: map[string][]string{
            "dashboard": {"templates/dashboard.go.html"},
            "users":     {"templates/users.go.html"},
        },
    },
}

users := htmx.ResourceHandlerFromCRUD[User, UserID](repo)
// This screen is read-only, even if the repository can write.
users.Create, users.Update, users.Destroy = nil, nil, nil
users.Views = htmx.ResourceViews{
    Page:  "users", // Template namespace, not a URL.
    Index: "users:list",
    Show:  "users:detail",
}

if err := app.Resource("users", users); err != nil {
    return err
}
if err := app.Page("/", "dashboard", "page"); err != nil {
    return err
}

// Mount supplies the base path for both routing and template URLs.
return app.Mount(router, "/admin")
```

`App.Page` stands for registering the full-document shell at an app-relative path. Its eventual data-loading API is left open. `ResourceViews` illustrates named fragment bindings; an optional callback can later handle cases that need a separate view model or response policy.

`templates/layout.go.html`:

```gotemplate
{{define "page"}}
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Users</title>
    <script defer src="{{htmxScript}}"></script>
  </head>
  <body>{{block "content" .}}{{end}}</body>
</html>
{{end}}
```

`templates/dashboard.go.html`:

```gotemplate
{{define "content"}}
  <h1>Users</h1>
  <section id="users"
           hx-get='{{endpoint "users"}}'
           hx-trigger="load"
           hx-swap="innerHTML">
    Loading users…
  </section>
  <section id="user-detail" aria-live="polite"></section>
{{end}}
```

`templates/users.go.html`:

```gotemplate
{{define "users:list"}}
<ul>
  {{range .}}
    <li>
      <button hx-get='{{endpoint "users" .ID}}'
              hx-target="#user-detail"
              hx-swap="innerHTML">{{.Name}}</button>
    </li>
  {{else}}
    <li>No users yet.</li>
  {{end}}
</ul>
{{end}}

{{define "users:detail"}}
  <article><h2>{{.Name}}</h2></article>
{{end}}
```

With the illustrative `/resources/<name>` route convention, mounting at `/admin` makes `{{endpoint "users"}}` return `/admin/resources/users`; passing an ID returns the item URL under it. Templates neither concatenate the base path nor know the resource's routing convention.

Rendering the shell does **not** call `FindAll`. The browser's later GET calls the resource's `Index`, checks iterator errors and renders the list fragment. The item GET calls `Show` and renders the entity fragment. In the direct case, the list template receives `[]User` as dot and the detail receives `User`: no mandatory DTO mapping or framework envelope.

This particular shell requires JavaScript to load its content. Progressive enhancement remains possible by rendering initial data on the server and providing real navigation URLs; async loading alone does not provide it. These example resource views are deliberately fragment-only, not history-navigation destinations (see §11).

Mount the app under the application's authentication, CSRF and other appropriate middleware. Every resource request must pass through that protection; protecting only the initial page is insufficient.

The lower-level `Handler`, `Render` and template components remain usable independently. `App` is the convenient registration layer, not a requirement for every HTML handler.

## 3. Incoming requests

```go
type Version uint8

const (
    VersionUnknown Version = 0
    Version2       Version = 2
    Version4       Version = 4
)

type RequestType uint8

const (
    RequestFull RequestType = iota
    RequestPartial
)

type Element struct {
    Raw string
    Tag string
    ID  string
}

type Request struct {
    HTMX           bool
    Version        Version
    Type           RequestType
    Boosted        bool
    HistoryRestore bool
    CurrentURL     string
    Target         Element
    Source         Element
}

func ParseRequest(r *http.Request) Request
func (r Request) IsPartial() bool
```

### Parsing rules

1. `HTMX` is true only for `HX-Request: true`. Without it, `VersionUnknown` and `RequestFull` apply, regardless of other HX headers.
2. An htmx request with `HX-Request-Type` or `HX-Source` is treated as v4-shaped; otherwise it is treated as v2-shaped. This is a compatibility heuristic, not proof of the installed client version.
3. History restoration always selects a full response.
4. For v4-shaped requests, only an explicit `HX-Request-Type: partial` selects a partial. Missing, malformed and unknown values select full. Explicit `full` wins even when other hints look like a targeted request.
5. For v2-shaped requests, choose partial only when neither `HX-Boosted` nor `HX-History-Restore-Request` is true. This is a convention, not a way to recognise every v2 `hx-select` request. Such applications should deliberately return full markup for those endpoints.
6. v4 source/target identifiers split at the **first** `#`; decode the ID with `url.PathUnescape`, not `QueryUnescape`. Preserve `Raw`. A missing `#` means a tag with no ID; invalid percent encoding leaves the parsed ID empty. v2 identifiers are bare IDs, with no URI decoding.
7. `CurrentURL` is the raw `HX-Current-URL` hint. No implicit fallback, URL validation or navigation is performed. Callers still have `r.URL` when they need the actual request target.

`IsPartial` is equivalent to `HTMX && !HistoryRestore && Type == RequestPartial`.

**Security boundary:** never use `Target`, `Source` or `CurrentURL` as authorisation, an unrestricted template/file name, or an outbound-fetch URL. Use application-owned mappings. A malformed optional element identifier does not turn a valid HTTP request into a 500.

No context accessor is initially necessary: parsing a handful of headers is inexpensive, and view models can explicitly carry any request details a template needs. Raw legacy headers such as `HX-Trigger-Name` remain accessible through `r.Header` without widening this model.

## 4. Renderable HTML

```go
type Component interface {
    Render(ctx context.Context, w io.Writer) error
}

type ComponentFunc func(ctx context.Context, w io.Writer) error

func (fn ComponentFunc) Render(ctx context.Context, w io.Writer) error

func Template(t *template.Template, name string, data any) Component
func Node(n interface{ Render(io.Writer) error }) Component
```

- `Component` has the same method set as `templ.Component`; the adapter does not import templ.
- `Node` adapts gomponents-style values. It cannot make a context-unaware renderer cancellable, but it can check cancellation before calling it.
- `Template` calls `ExecuteTemplate` on an existing stdlib template. It does not mutate the template, install funcs or change options. An empty name selects the template's own name.
- A component knows nothing about HTTP headers, status, redirects or htmx versions. The writer need not be an `http.ResponseWriter`.
- Components may produce partial output before returning an error. HTTP-level buffering is the responsibility of `Render`, not each component.
- Template execution respects cancellation before/after execution; arbitrary template funcs cannot be forcibly interrupted. Request-scoped funcs must observe their own context.

This also allows completely manual rendering:

```go
component := htmx.Template(existingTemplates, "users:list", data)
err := component.Render(ctx, &buf)
```

## 5. Stdlib template sets

```go
type TemplateConfig struct {
    FS     fs.FS
    Shared []string
    Pages  map[string][]string
    Funcs  template.FuncMap
}

type Templates struct {
    // Unexported, immutable parsed template sets.
}

func ParseTemplates(config TemplateConfig) (*Templates, error)
func (ts *Templates) Execute(page, name string, data any) Component

type TemplateSample struct {
    Page string
    Name string
    Data any
}

func (ts *Templates) Check(ctx context.Context, samples ...TemplateSample) error
func FuncMap() template.FuncMap
```

### Loading and naming

- `Shared` and each `Pages` value contain `fs.Glob`-style patterns. `**` has no special recursive meaning. Paths are relative to `FS`, with no leading slash.
- Initially require explicit page registration. A project can follow `layouts/`, `partials/`, `pages/` conventions without a second, implicit discovery system in the library.
- Page keys such as `users/index` identify **template namespaces**, not routes or filenames. Each namespace contains the shared templates plus its own page files. `App.Page` registers an HTTP page separately; its use of a namespace does not change this distinction.
- Parse shared templates once, then clone that unexecuted set for each page and parse its files. Explicit page overrides of shared definitions are allowed. Glob matches have deterministic lexical order; explicit pattern order is preserved. Other accidental duplicate definitions should be reported, not selected by map iteration order.
- Use explicit `{{define}}` names. `Execute("users/index", "page", data)` and `Execute("users/index", "users:list", data)` use the same namespace. There is no separate rendering engine for fragments.
- Empty configuration, missing files, parse errors and unresolved template references fail construction. Errors identify the page namespace and file where available.
- Default to `Option("missingkey=error")`. This covers missing map keys accessed through ordinary template field lookup; it is not a universal check for every function or `index` expression.
- `Execute` returns a component; an unknown page or definition becomes an error when that component renders. Applications should cover their fixed selections with `Check` at startup or in tests.

### Lifecycle

`ParseTemplates` publishes an immutable snapshot. Copy configuration maps/slices during construction. No later `Parse`, `Funcs` or `Clone` is performed on a published page set. Concurrent execution uses independent writers; user-supplied funcs and data must themselves be safe to share.

When used through `App`, mounting first establishes the resource registry and public base path, then supplies mount-bound functions such as `endpoint` before parsing/publishing the app's template snapshot. Do not patch live template funcs on each request or capture a mutable global "current app". Standalone `ParseTemplates` remains independent of resource registration.

**Correction to the research sketch:** executing a page clone does not prevent cloning an untouched shared base. Eager construction is chosen for predictable startup failures and no request-time parsing, not because executing any clone invalidates the original base.

For development, use `os.DirFS`, rebuild a complete snapshot when files change, and replace the active snapshot at the application boundary. A built-in watcher or `Reload bool` is deferred. Production can use `embed.FS` unchanged.

### Checks are execution tests, not static typing

```go
err := views.Check(ctx,
    htmx.TemplateSample{Page: "users/index", Name: "page", Data: emptyPage},
    htmx.TemplateSample{Page: "users/index", Name: "users:list", Data: populatedPage},
)
```

`Check` renders the supplied samples into `io.Discard` and aggregates failures with page/definition context. It checks only the branches those samples exercise. It may call funcs or consume iterators; fixtures should be deterministic and side-effect free. It does not replace templ's compile-time checking.

**Iterator caveat:** a value of type `iter.Seq2[T, error]` does not automatically return its yielded error from `ExecuteTemplate`. Collect and check it in the handler/use case, or explicitly translate iteration errors into template execution errors. Buffering alone cannot detect an ignored iterator error.

### Default template functions

`FuncMap` returns a fresh map. `ParseTemplates` installs it before adding application funcs. Reject collisions with these reserved names rather than silently replacing safety-sensitive functions.

| Template name | Signature                                               | Contract                                                                                                                                         |
| ------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `json`        | `func(any) (string, error)`                             | `json.Marshal` as ordinary text; normal HTML escaping still applies. Never returns `template.JS` or `template.HTML`.                             |
| `path`        | `func(base string, segments ...string) (string, error)` | Validates a server-owned, root-relative base; rejects empty/`.`/`..` segments; escapes each segment with `url.PathEscape` before `pathkit.Join`. |
| `dict`        | `func(...any) (map[string]any, error)`                  | Alternating string keys and values. Reject odd length, non-string keys and duplicate keys.                                                       |

`path` accepts a path prefix, not an arbitrary URL: reject schemes, authorities (`//host`), queries, fragments, backslashes and control characters in the base. Dynamic inputs are raw segments, not already percent-encoded strings. Query strings belong in `url.Values`, outside this helper. Escaping is not authorisation and does not compensate for downstream double-decoding of paths.

Safe examples:

```gotemplate
<button hx-post='{{path "/users" .ID "activate"}}'
        hx-vals='{{json .ExtraValues}}'>Activate</button>

{{template "user:card" (dict "User" .User "Editable" .CanEdit)}}
```

For inline JavaScript, `json` must occupy a **whole value position**, e.g. `hx-on:click="choose({{json .ID}})"`, not a value nested inside an already-quoted JS string. Prefer external event handlers with data attributes when feasible. These funcs do not make arbitrary htmx/Alpine expressions safe.

`App` additionally supplies mount-bound `endpoint` and `htmxScript` helpers (§11). These are not part of the standalone `FuncMap`, because they need a particular app's registrations and public base path. General asset helpers remain application-specific; this does not make the template package own an asset pipeline.

## 6. HTTP response values and headers

```go
type View struct {
    Full    Component
    Partial Component
}

type Response struct {
    StatusCode int
    View       View
    Headers    ResponseHeaders
}

type ResponseHeaders struct {
    Redirect   string
    Location   string
    Refresh    bool
    PushURL    string
    ReplaceURL string
    Retarget   string
    Reselect   string
    Reswap     Swap
    AfterSwap  []Event
}

type Event struct {
    Name   string
    Detail any
}

type Swap string

const (
    SwapDefault     Swap = ""
    SwapInnerHTML   Swap = "innerHTML"
    SwapOuterHTML   Swap = "outerHTML"
    SwapBeforeBegin Swap = "beforebegin"
    SwapAfterBegin  Swap = "afterbegin"
    SwapBeforeEnd   Swap = "beforeend"
    SwapAfterEnd    Swap = "afterend"
    SwapDelete      Swap = "delete"
    SwapNone        Swap = "none"
)

func (h ResponseHeaders) Apply(dst http.Header, version Version) error
```

### Header rules

- Empty fields mean no instruction; `Refresh: false` means no refresh. `PushURL`/`ReplaceURL` may be `"false"` to suppress a client-side history action.
- `Redirect`, `Location` and `Refresh` are mutually exclusive. Navigation must not be combined with swap, history or event instructions. `PushURL` and `ReplaceURL` are also mutually exclusive.
- Built-in swap constants deliberately cover the shared vocabulary. `Swap` also accepts an explicit raw string for advanced callers; raw modifiers are **not translated** between versions. A comprehensive typed modifier builder is deferred until the v2/v4 browser matrix is tested.
- `AfterSwap` requests events after a successful swap. Encode it as `HX-Trigger-After-Swap` for v2 and `HX-Trigger` for v4. Delivery is conditional on client handling, not a portable event-only-response mechanism. `Render` rejects these events on bodyless responses or with the `none` swap style; v2 error responses still require client configuration to permit swapping. Do not promise identical settling/animation timing. No API for the removed v4 after-settle header.
- Encode events into one JSON object. Empty or duplicate event names fail. Nil detail encodes as `null`; other details must be JSON-marshalable. Callers combine events before rendering; no hidden merge with arbitrary existing JSON headers.
- `Apply` accepts only `Version2` or `Version4` and validates/serializes all instructions before touching `dst`. `VersionUnknown`, other invalid versions, conflicts, invalid JSON and header control characters return errors with no partial header update.
- On success, the supplied value is authoritative for the library-owned HX response headers: clear their previous values, including old event-timing variants, then set the new ones. Preserve unrelated headers. Do not modify the source value or its referenced data.
- `Apply` cannot enforce status/body rules because it receives only `http.Header`. Callers using it directly must apply it before writing and obey the navigation contract below. Most users should call `Render` or use `Handler`.

Maps inside `Detail` and slices inside `ResponseHeaders` remain reference-backed. Copying a response struct does not make those values independently mutable or safe for concurrent changes.

## 7. Rendering and commit semantics

```go
func Render(w http.ResponseWriter, r *http.Request, response Response) error

type RenderError struct {
    Err       error
    Committed bool
}

func (e *RenderError) Error() string
func (e *RenderError) Unwrap() error
```

### Selection

| Request/response                          | Behaviour                                                                         |
| ----------------------------------------- | --------------------------------------------------------------------------------- |
| Ordinary request or full htmx request     | Execute `View.Full`.                                                              |
| Partial htmx request                      | Execute `View.Partial`.                                                           |
| Selected component is nil                 | Pre-commit configuration/render error; never substitute the other representation. |
| Same content works in either context | Explicitly assign that component to both fields. This also supports an intentionally fragment-only resource endpoint. |
| Status 204, 205 or 304                    | No view; reject a response containing either component. No `AfterSwap` events.    |
| Zero status, no navigation                | Status 200.                                                                       |
| Redirect, location or refresh instruction | Use the navigation rules in §8; no view.                                          |

A full request with no full representation is not automatically a 404: the handler must choose an appropriate application response rather than letting fragment selection make a routing decision.

For non-navigation responses, accept final 2xx/4xx/5xx statuses and 304; reject unsupported status combinations, including 1xx and ordinary 3xx. Status 205, like 204, must not contain response content. Prefer 204 when the intent is explicitly to prevent an htmx swap. Custom HTTP behaviour remains available through a plain handler.

For **ordinary requests**, validate response values but omit non-navigation HX instructions from the emitted headers. Do not call `Apply` with `VersionUnknown` or fabricate a client version. Navigation becomes a normal HTTP redirect as described below. On success, clear stale library-owned HX response headers in this branch too; on failure, leave the writer untouched.

### Atomic preparation, then a single commit

1. Parse request hints and validate the response.
2. Prepare/serialize the response headers in temporary storage.
3. Render the selected component into a private buffer. No success-only HX header or status is written yet.
4. Set `Content-Type: text/html; charset=utf-8` for HTML bodies, merge cache metadata, apply the prepared headers, and write the status.
5. Write the body. `HEAD` performs the same preparation but does not write body bytes; bodyless statuses likewise have no body.

An error before step 4 returns `*RenderError{Committed: false}` without changing the response writer's headers or body. A write failure after step 4 returns `*RenderError{Committed: true}`. There is no rollback after an HTTP response has been committed.

Callers must not have written/flushed before calling `Render`. Otherwise its pre-commit guarantee cannot apply. A network write failure is not an invitation to append an error page.

No streaming is promised by this API. Large exports and SSE use separate handlers. Initial buffering uses memory; a configurable size limit or spooling policy should be considered before exposing this for unbounded responses.

### Cache correctness without mandatory middleware

`Render` adds the representation-dependent `Vary` fields to both full and partial replies:

```text
HX-Request, HX-Request-Type, HX-Boosted, HX-History-Restore-Request, HX-Source
```

Merge case-insensitively with existing values; preserve `Vary: *`. The additional fields are intentional: v2 selection depends on boost/history, and the proposed version heuristic consults source. Varying on source is conservative and may reduce cache reuse.

If application code chooses content based on `HX-Target`, cookies, locale or other inputs, it owns the corresponding cache policy too. Personalised pages usually need `private` or `no-store`; `Vary` is not an access-control mechanism. Strong ETags must describe the selected representation, not a different full/partial body.

## 8. Redirects and navigation

```go
func Redirect(location string) Response
func Location(location string) Response
```

These are convenience constructors for `Response{Headers: ...}`. The caller still returns the response through `Handler` or passes it to `Render`.

| Instruction                      | Ordinary request                              | htmx 2-shaped request            | htmx 4-shaped request    |
| -------------------------------- | --------------------------------------------- | -------------------------------- | ------------------------ |
| `Redirect("/users")`             | 303 + `Location`                              | 204 + `HX-Redirect`              | 204 + `HX-Redirect`      |
| `Location("/users")`             | 303 + `Location`                              | 204 + **`HX-Redirect` fallback** | 204 + `HX-Location`      |
| `ResponseHeaders{Refresh: true}` | 303 to the current request's local path/query | 204 + `HX-Refresh: true`         | 204 + `HX-Refresh: true` |

Require `StatusCode == 0` and no view for these response values; `Render` chooses the protocol-appropriate status. Never put navigation HX headers on a 3xx response.

The v2 `Location` fallback is deliberate: v2 cannot reliably distinguish the follow-up full-page request from an ordinary fragment request. JSON-form `HX-Location` options are deferred.

Navigation/history destinations must be local, root-relative paths, optionally with query/fragment. Reject an authority, scheme, backslash, control characters or malformed escapes; validation must account for URL spellings browsers normalise into a network-path reference. Do not derive a destination blindly from `HX-Current-URL`. Applications needing approved external redirects can use a plain handler with their own allowlist.

The 303 fallback implements POST/Redirect/GET. This API does not try to express every HTTP redirect status or method-preservation policy.

## 9. A handler adapter, not a controller framework

```go
type Handler struct {
    Handle       func(r *http.Request) (Response, error)
    ErrorHandler httpkit.ErrorHandler
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

- Invoke `Handle`, then call `Render`. A nil `Handle` is a server configuration error, not a panic.
- On a use-case error or a pre-commit rendering error, invoke `ErrorHandler`; a nil value uses `htmx.ErrorHandler{}`.
- On a committed rendering error, log it through the project's logging facilities and stop. Never call an error renderer after commitment.
- Do not silently recover arbitrary application panics. Recovery/transaction policy belongs to surrounding middleware.
- The callback has no response writer, so it cannot accidentally commit before returning its response. Use a plain handler where direct writer access is needed.

For forms, keep validation errors as real error statuses and preserve the submitted view model:

```go
return htmx.Response{
    StatusCode: http.StatusUnprocessableEntity,
    View: htmx.View{
        Full:    views.Execute("users/edit", "page", form),
        Partial: views.Execute("users/edit", "user:form", form),
    },
}, nil
```

This is an intentional 422 response, not an unhandled rendering error. htmx 4 displays it by default. **htmx 2 requires explicit client response-handling configuration** to swap 422; response headers alone do not enable error-body swaps. The package must document a v2 recipe rather than change the response to 200.

Use the existing `formurlencoded.Codec` with bounded body reads for form input. The proposed resource adapter can reuse that decoding path; the exact input-mapping and post-mutation rendering hooks remain open. A general form framework, method override, validation framework and form component library are separate concerns.

## 10. HTML error handling

```go
type ErrorHandler struct {
    Mapping rfc7807.HandlerMappingFunc
    View    func(ctx context.Context, problem rfc7807.DTO) View
    Target  string
}

func (h ErrorHandler) HandleError(w http.ResponseWriter, r *http.Request, err error)
```

Proposed behaviour:

1. Reuse `rfc7807.Handler.ToDTO` for the base problem. Always apply `httpkit.ErrorMapping`, then the optional `Mapping` hook. This draft's hook **extends** the defaults, unlike assigning a replacement mapping directly on `rfc7807.Handler`.
2. Keep the existing statuses: `UserError` defaults to 400; not-found/forbidden/conflict retain their established mappings. Do not globally remap every user error to 422.
3. `View` may select application templates. If nil, supply built-in minimal full-page and partial error markup containing the status and escaped title, not a raw `err.Error()`, stack trace, or arbitrary problem detail.
4. For partial requests, retarget to `Target` (default `body`) with an `innerHTML` swap. The default partial is body content, not a nested document. Applications can choose an error region instead. Status stays 4xx/5xx, with the same v2 client-configuration caveat as above.
5. Request cancellation produces no new response when the operation error also represents cancellation.
6. A `RenderError` with `Committed == true` is logged but not rendered again. Custom error handlers used directly must follow the same rule.
7. If the custom error view fails before commitment, log the failure and send a fixed, minimal 500 response. Do not recursively invoke the same broken view. Do not retain success-only navigation/trigger instructions.

Problem `Detail` may contain developer-facing information; rendering it to users is an explicit application decision. Localisation should use stable problem IDs, not error string matching.

`ErrorHandler` can be passed to `httpkit.TransactionMiddleware` as well. A commit failure must not leak the buffered success body, redirect or events; testing that integration is required before claiming transactional response safety. This type does not automatically replace `httpkit.Router`'s own default 404/405 responses.

## 11. `App`: named resources and a mount-bound web UI

**High-level direction, not a finished type hierarchy.** `App` coordinates the parts that need to agree on routes and URL generation. It does not own business dependencies, an HTTP server or a background runtime.

### 11.1 Responsibilities

| Piece | Responsibility |
| --- | --- |
| CRUD resource or application service | Data access, business rules and user/tenant scoping. |
| `ResourceHandler[ENT, ID]` | REST-shaped operation dispatch, bounded input decoding, error handling and selection of HTML views. |
| Resource view binding | Turn an operation result into an entity/list template, or explicitly map it to a custom view model/response. |
| `App` | Register resources under stable names, register page routes, bind templates and asset URLs to a mount. |
| Template `endpoint` helper | Resolve a registered name and optional ID to a URL; never fetch the resource itself. |
| Host router and middleware | Mount the app, authenticate every request, apply CSRF protection, transactions and other HTTP policies. |

This is the **`httpkit.RESTHandler` idiom with HTML presentation**, not automatic admin-screen generation. A reusable renderer remains available below it. Reuse httpkit's routing and existing CRUD/error/form conventions where possible; do not pretend a request-unaware `codec.Marshaler` is enough to express HTML navigation and fragment selection.

### 11.2 Two ways to supply a resource

**Direct wiring:** `ResourceHandlerFromCRUD[ENT, ID](resource)` fills the corresponding operation fields from the resource's supported ports. The application adds named templates and registers the result. It should not need a mandatory DTO mapper, a new all-purpose repository interface or a wrapper class just to list and show entities.

**Application-specific wiring:** construct the same handler with functions, or replace individual fields supplied by the convenience constructor. The intended operation shapes are familiar:

```go
// Sketch of the operation surface; not the final complete struct.
type ResourceHandler[ENT, ID any] struct {
    Index   func(context.Context) iter.Seq2[ENT, error]
    Show    func(context.Context, ID) (ENT, bool, error)
    Create  func(context.Context, *ENT) error
    Update  func(context.Context, *ENT) error
    Destroy func(context.Context, ID) error

    Views ResourceViews // Named view bindings; exact shape remains open.
}
```

| Relative request | Operation | Direct CRUD source |
| --- | --- | --- |
| `GET /`, `HEAD /` | `Index` | `crud.AllFinder[ENT].FindAll` |
| `GET /:id`, `HEAD /:id` | `Show` | `crud.ByIDFinder[ENT, ID].FindByID` |
| `POST /` | `Create` | `crud.Creator[ENT].Create` |
| `PUT /:id` | `Update` | `crud.Updater[ENT].Update` |
| `DELETE /:id` | `Destroy` | `crud.ByIDDeleter[ID].DeleteByID` |

As with `RESTHandler`, a nil operation is unavailable (405 on its recognised route). Supported methods are not a permission grant: every operation still enforces the application's access policy. The convenience constructor must document capability discovery, and callers must deliberately restrict exposed operations. In particular, a repository implementing `DeleteAll` or `Purge` must not silently acquire a bulk-delete HTTP endpoint. `Saver` is not silently treated as an updater, and `PUT` remains replacement rather than an invented partial update.

Discovering a write method does not, by itself, make a mutation endpoint ready. Its input/access policy and a valid success/error response policy must be established, whether explicitly or through documented defaults. Otherwise keep it unavailable or reject setup; do not first persist a change and only then discover that no response is configured.

For `PUT /:id`, the path selects the entity being authorised and updated. Submitted fields must not redirect the update to a different entity ID. Whether a conflicting body ID is rejected or replaced by the path ID is a later policy choice, but authorisation and mutation must concern the same record.

For user-specific results, the same registration can instead use:

```go
users := htmx.ResourceHandler[User, UserID]{
    Index: service.ListVisibleUsers,
    Show:  service.FindVisibleUser,
    Views: htmx.ResourceViews{
        Page:  "users",
        Index: "users:list",
        Show:  "users:detail",
    },
}
if err := app.Resource("users", users); err != nil {
    return err
}
```

Here the application's methods have the shown CRUD-compatible signatures, read the authenticated principal from `ctx`, and perform scoped queries. They can call a repository, combine results, or perform additional logic without the template knowing which implementation is in use. A decorator that itself implements CRUD ports can also go through `ResourceHandlerFromCRUD`.

`CollectionContext`, `ResourceContext` and `Filters` are useful precedents from `RESTHandler`; their exact HTML-adapter surface is not settled here. Prefer scoped queries for large or paginated lists. A post-fetch list filter alone does **not** protect `Show`, `Update` or `Destroy`, nor does hiding a button. Counts, pagination and personalised caching must respect the same scope. Query-specific HTTP input can be mapped at this edge without pushing HTTP concerns into domain APIs.

### 11.3 Minimal view mapping, with an escape hatch

The simple path binds each read operation to a named template:

- `Index` → a collection template with `[]ENT` as dot.
- `Show` → an entity template with `ENT` as dot, only when `found` is true. `found == false` becomes a 404 through the shared HTML error handling, never a successful zero-valued entity view.
- Normalise CRUD absence/conflict errors to the corresponding HTTP errors/statuses, as `RESTHandler` does (for example 404 and 409), before using the shared error renderer.
- Resource IDs follow the same entity/ID conventions as the CRUD ports. Incoming ID parsing and outgoing ID formatting must agree; the endpoint helper receives raw IDs, not already-escaped paths.

The adapter, not the template author, must consume `iter.Seq2[ENT, error]` correctly and propagate failures before sending a success response. The illustrative list binding collects a bounded/page-sized result before rendering; it is not permission to load an unbounded table. Pagination and the precise list-size limit remain design questions.

An optional callback can later support joins, presentation-specific view models, status/trigger selection and custom components. **That callback should be optional for ordinary entities and lists.** Input mapping for writes is a separate concern: bounded form decoding can reuse `formurlencoded`, but writable fields, ownership and other server-managed values must be explicitly controlled.

For successful mutations, the application may want an updated item fragment, a refreshed list, an empty swap, a trigger, or a redirect. Keep those policies explicit; this iteration does not fix a universal response template or copy RESTHandler's bodyless update response blindly. Validation should be able to re-render submitted values without persisting invalid data.

There are two legitimate endpoint shapes:

1. **Page-capable resource:** register both full and partial views and use the request selection described earlier. URLs pushed into browser history must work as full-page navigation.
2. **Intentionally fragment-only resource:** declare that it always returns resource markup. The simple `Index`/`Show` view bindings in §2 illustrate this choice; internally both `View` branches can intentionally select the same fragment. These URLs are not automatically page-navigation destinations, and their absence of a full document is not a missing-view error.

Do not infer which shape was intended from a missing view or from a particular DOM target. The precise configuration syntax for choosing between them remains open.

### 11.4 Registration names and template endpoints

The registry associates a **logical name** (for example `"users"`) with a resource handler, app-relative routing metadata and view bindings. A name is not a raw URL or a template filename. The `/resources/users` convention in the example is only an initial route convention; changing it should not require editing every template.

Conceptually, the mount-bound helpers behave as follows:

| Template call | Meaning | Example with the app mounted at `/admin` |
| --- | --- | --- |
| `{{endpoint "users"}}` | Collection endpoint | `/admin/resources/users` |
| `{{endpoint "users" .ID}}` | Item endpoint, using that resource's ID formatting | `/admin/resources/users/u-42` |
| `{{htmxScript}}` | Bundled htmx script URL for this mount | A content-hashed URL below `/admin/_htmx/` |

The HTTP method comes from `hx-get`, `hx-post`, `hx-put` or `hx-delete`; the helper need not invent separate collection URLs for reading and creating. Nested resources and named custom actions can be designed later rather than overloading this initial helper with arbitrary path fragments.

The important boundaries are:

- Resolve against the registration table, not by concatenating whatever resource name came from user input. Duplicate names/route conflicts are setup errors; an unknown name or invalid ID is an error, never a fallback URL.
- Return an ordinary string (with an error where needed), allowing `html/template` to retain normal escaping. Do not return `template.HTML` or make trust assertions about arbitrary URLs.
- Validate dynamic segments and escape them exactly once before joining to the registered path with `pathkit`. Support the resource's legitimate ID kinds, including zero-valued integer IDs; reject empty or traversal-only string segments.
- The helper neither calls the repository nor authorises access. A URL may be generated for an item the subsequent request is not allowed to read; that request still performs its own checks.
- Fix endpoint functions to a particular mounted app. Two apps registering `"users"` must not share a global resolver or change one another's links.

### 11.5 Mounting establishes the base path

Retain the existing `App` name and mount concept, but no longer interpret it as an assets-only adapter. A candidate configuration is:

```go
// Conceptual fields; registration storage and binding types are omitted.
type App struct {
    Templates  TemplateConfig
    Version    Version
    MountPoint string // Optional browser-visible base override when it cannot be inferred.
}
```

The desired ordinary case is:

```go
// Register pages/resources without knowing their deployment prefix.
if err := app.Resource("users", users); err != nil {
    return err
}
return app.Mount(router, "/admin")
```

Mounting should establish one consistent view of:

- Where the host mux dispatches requests to the app.
- The app-relative page/resource/asset routes.
- The browser-visible base used by `endpoint`, script URLs and app-generated navigation.

The conceptual lifecycle is:

1. Register/configure resources and pages before serving.
2. At mount time, determine the public base and validate registrations and view references. Bind endpoint functions, then prepare the immutable templates and routing snapshot.
3. Publish the mounted handler only after preparation succeeds. Runtime lookup failures still report errors; mounting cannot prove every dynamically selected template resource name is valid.
4. Treat the mounted configuration as frozen. Late registrations or changes to its base path should fail rather than race with requests.

Use the routing/mount facilities in httpkit for dispatch; resource handlers should see their relative paths, and ID segments must survive escaping and decoding without double stripping.

There is an important limit to automatic base-path discovery: `httpkit.Multiplexer` only exposes `Handle`. An arbitrary mux cannot reveal its parent namespace or a reverse proxy's external prefix. Therefore:

| Deployment | Mux-local mount | Browser-visible app base |
| --- | --- | --- |
| Root router | `/admin` | `/admin`, inferred from `Mount` |
| Router already mounted at `/tools` | `/admin` | `/tools/admin`, supplied by trusted configuration or known routing context |
| Proxy strips `/console` before forwarding to the app | `/` | `/console`, explicitly configured |

In the latter cases an override such as `MountPoint` supplies the **complete public app base**, not an extra prefix to add a second time. Never guess it from `HX-Current-URL`, `Host`, or untrusted forwarded headers. `/_htmx` is now an internal asset route, not the default public base of the entire app.

The simplest initial lifecycle is one bound mount per `App`. Alternatively `Mount` could produce separate immutable mounted instances. **Leave that return type and receiver choice open**, but never let a second mount overwrite the first mount's endpoint functions. Endpoint evaluation before binding must not produce silently root-relative, incomplete URLs.

The illustrative examples use an error-returning `Mount` to surface setup failures; changing the current stub's signature, choosing a pointer receiver, or introducing a separate `Build`/mounted value is a later API decision. Ordinary lower-level handlers and templates must remain usable without this registration layer.

### 11.6 Browser assets remain a supporting concern

The bundled script should be served under the app's base, with content-hashed URLs and immutable cache headers. Retain the earlier goals of explicit pinned versions/licences, no runtime CDN download, GET/HEAD support and correct content types. `htmxScript` resolves through the same mounted configuration as resource endpoints.

Client configuration remains ordinary markup, for example `<meta name="htmx-config" content='{{json .HTMXConfig}}'>`. A typed mirror of every setting, extension bundling and a general asset pipeline remain deferred. The selected bundled client version does not turn request-header inference into trusted protocol negotiation.

## 12. Test support

Proposed subpackage: `go.llib.dev/frameless/adapter/htmx/htmxtest`.

```go
func WithRequest(r *http.Request, hx htmx.Request) *http.Request
```

`WithRequest` clones the request/header map, clears existing library-owned HX request headers, then encodes the supplied model. `HTMX: true` requires an explicit supported version; invalid or unrepresentable test configuration panics, following `httptest.NewRequest`'s programmer-error convention. The generated hints must round-trip through `ParseRequest` for the supplied normalized fields:

- A v2 full request requires boost or history restoration; a v2 partial request requires neither. `Version2` plus zero-valued `RequestFull` alone is not representable and panics.
- A history-restore request must be `RequestFull` in either version.
- v2 element identifiers must omit `Tag`; v4 elements with an ID must supply `Tag`. A zero `Element` emits no identifier header.
- `HTMX: false` requires otherwise zero-valued hints and produces an ordinary request with no generated HX headers.

The original request is unchanged.

```go
req := htmxtest.WithRequest(
    httptest.NewRequest(http.MethodGet, "/users", nil),
    htmx.Request{
        HTMX:    true,
        Version: htmx.Version4,
        Type:    htmx.RequestPartial,
        Target:  htmx.Element{Tag: "ul", ID: "users"},
    },
)
rr := httptest.NewRecorder()
handler.ServeHTTP(rr, req)
```

For generated identifiers, use `Tag`/`ID`, not `Raw`. The helper must follow the client header encoding, including v4's URI encoding rather than treating IDs as query values. Handwrite malformed headers when testing invalid input. Do not create a second assertion framework; use testcase assertions on `rr.Code`, `rr.Header()` and the rendered body, and `Templates.Check` for fixtures.

### Behaviour to pin before implementation is accepted

- Plain, v2 partial/boosted/history and v4 partial/full/history requests, including unknown types and malformed IDs.
- `Vary` merging and cache separation across all headers that influence selection/version.
- Missing selected views, unknown template names and JSON serialization failures remain pre-commit failures.
- Writer failures after commitment never append an error page.
- Same page namespace can render full and fragment output concurrently without per-request template mutation.
- Safe JSON-value and path-segment examples, plus negative cases for inline JS, traversal, redirects and malformed URLs.
- Navigation statuses/headers for both clients and no-JS requests.
- v2 after-swap versus v4 trigger headers; preserve meaningful 422/500 statuses.
- Resource collection/item endpoints dispatch to the configured CRUD-shaped operations; disabled methods remain unavailable.
- Direct CRUD wiring and a user-scoped application layer can use the same named templates and registration.
- Scoping applies to individual reads and mutations as well as lists; late iterator errors cannot produce a truncated success response.
- `endpoint` resolves collection and typed-ID item URLs without loading data, including IDs requiring escaping.
- Changing the app mount changes generated resource and script URLs together; namespace/proxy overrides do not double-prefix URLs.
- Duplicate registrations, unknown resource names and lookup before base-path binding produce errors. Independent apps cannot leak endpoint bindings into one another.
- Asset mounting under the app prefix, content hashes, GET/HEAD and immutable cache headers.
- Transaction commit failures discard success response instructions as well as the success body.

HTTP-level tests do not establish browser swap semantics. Add a small browser compatibility suite against the pinned client bundles before promising OOB translation, advanced swap options or multi-target helpers.

## 13. Deliberately deferred APIs

| Possible addition                               | Why not in the first public surface                                                                                                                                                                              |
| ----------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Typed `Partial` / multi-target response builder | v4 `<hx-partial>` and v2 OOB swaps differ in order, wrapping and table/SVG parsing. Do not pretend there is a universally correct automatic translation. Authors can use native markup within a component today. |
| Full typed swap-modifier builder                | Version-specific syntax and default/false/unset semantics deserve browser-backed tests. Shared styles and explicit raw syntax suffice initially.                                                                 |
| Explicit protocol-profile override              | Useful for nonstandard clients or migration deployments, but header inference plus raw handlers covers the initial conventional clients. Add only with a documented negotiation contract.                        |
| Header constants for the complete protocol      | A constants catalogue adds surface without solving the main template/rendering problem; raw `http.Header` remains available.                                                                                     |
| Request middleware/context accessors            | Not needed for correctness; add later only if consumers demonstrate a need. No context-held response accumulator.                                                                                                |
| Automatic template discovery / live reload      | Explicit patterns and immutable snapshots keep lifecycle predictable; development tools can rebuild snapshots.                                                                                                   |
| General form framework / automatic admin UI | Resource registration and CRUD-shaped operations are in scope. Automatically generated forms, inferred business permissions and a universal mutation UX are not. |
| Nested resource/action URL DSL | Start with named collection/item endpoints. Design parent scopes, custom action naming and path overrides after the simple registration model is reviewed. |
| SSE/pub-sub integration                         | Different lifetime, flushing and cancellation contract from buffered page responses.                                                                                                                             |
| Template linting                                | A template parse tree alone does not provide complete HTML/JS context analysis. Avoid advertising a partial check as a security guarantee.                                                                       |

## 14. Decisions to review before implementation

### Direction agreed by this revision

1. **`App` registers a resource-backed web UI.** It coordinates routes, template endpoint lookup and assets, without owning business services or the HTTP server.
2. **Resources follow the RESTHandler idiom.** CRUD ports can supply operations with little mapping; custom functions or a scoped CRUD decorator can add application logic.
3. **Templates refer to names, not deployment paths.** Endpoint lookup only produces URLs. The subsequent HTTP request performs data access and access checks.
4. **Mounting establishes the base before serving.** The registry, public paths and template funcs must describe the same mounted instance.
5. **Rendering remains explicit and independently usable.** Choose page-capable versus fragment-only endpoints deliberately; do not make a missing full view silently mean something else.
6. **The earlier response safety rules still apply.** Buffered preparation, commit-aware errors and conservative v2/v4 compatibility support the resource layer too.

### Still open at this high-level stage

- Should registration return a resource handle, take a string name, or support both? How does a custom handler attach equivalent endpoint metadata?
- What is the smallest `ResourceViews` shape for collection/entity templates, optional full pages, and custom view-model/response callbacks?
- Which operations should `ResourceHandlerFromCRUD` enable by default, and how should a registration explicitly restrict the surface? Bulk deletion must not be inferred automatically.
- Should mounting bind one `App` once, return a mounted instance, or use a separate preparation step? How should trusted parent/proxy prefixes enter that API?
- How do mutation results, validation redisplay, paging and bounded iterator consumption fit without imposing a form framework?
- How should shared error handlers and middleware be configured for all registered endpoints while leaving per-resource customisation possible?

Exact exported registration interfaces, receiver types, field names and lifecycle machinery should wait. Review the model first with two examples—one directly backed by CRUD ports and one scoped to the current user—mounted under different prefixes. Both should use the same endpoint helper and templates without embedding deployment paths or authorisation rules in HTML.
