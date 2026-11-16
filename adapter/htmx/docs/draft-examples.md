# Draft examples: possible `htmx` API styles

**These are design examples, not implemented APIs.** They illustrate alternative ways to satisfy [public-api-draft.md](public-api-draft.md), especially its resource-registration and mounting requirements. They are not implementation instructions, compatibility promises, or programs expected to compile against the current package.

The alternatives deliberately hold the resource behaviour and HTML mostly constant, while changing **how the application is assembled and when its URLs become bound**. They need not all be implemented. Names introduced here, such as `ResourceRegistration`, `ResourceRef` and `MountedApp`, are illustrative.

| Candidate | Style | Main appeal |
| --- | --- | --- |
| [A. Register, then mount](#a-register-then-mount) | Struct fields and explicit registration methods | Closest to `httpkit.RESTHandler` and the current draft; little machinery. |
| [B. Declare the app in one value](#b-declare-the-app-in-one-value) | Resource/page registration slices | The whole composition is visible in one configuration value. |
| [C. Register typed references, then create mounted instances](#c-register-typed-references-then-create-mounted-instances) | Typed resource handles and a separate bound lifetime | Useful for Go-side URL generation and mounting one definition at several prefixes. |

## Shared scenario and conventions

A small directory UI has:

- A page shell that loads a list asynchronously.
- A named `"users"` resource with collection and item GET endpoints.
- A list fragment and a detail fragment written with stdlib `html/template`.
- Either a CRUD-compatible repository or an application layer that restricts results to the current user.
- A mount prefix chosen by the composition root, not embedded in the HTML.

Imports and unrelated application/server code are omitted. Each candidate is a separate alternative, not a series of registrations to run together.

### Application types and templates

```go
type UserID string

type User struct {
    ID   UserID `ext:"id"`
    Name string
}

//go:embed templates/*.go.html
var templateFS embed.FS

func directoryTemplates() htmx.TemplateConfig {
    return htmx.TemplateConfig{
        FS:     templateFS,
        Shared: []string{"templates/layout.go.html"},
        Pages: map[string][]string{
            "directory": {"templates/directory.go.html"},
            "users":     {"templates/users.go.html"},
        },
    }
}

func userFragments() htmx.ResourceViews {
    return htmx.ResourceViews{
        Page:  "users", // Template namespace, not the resource's URL.
        Index: "users:list",
        Show:  "users:detail",
    }
}
```

For these examples, `ResourceViews`' short string bindings **explicitly mean fragment-only resource views**. They return resource markup for ordinary and htmx requests alike; they do not silently substitute fragments for missing full-page views. A page-capable resource would need an explicit full/partial binding instead.

`templates/layout.go.html`:

```gotemplate
{{define "page"}}
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>User directory</title>
    <script defer src="{{htmxScript}}"></script>
  </head>
  <body>{{block "content" .}}{{end}}</body>
</html>
{{end}}
```

`templates/directory.go.html`:

```gotemplate
{{define "content"}}
  <h1>User directory</h1>

  <section id="users"
           hx-get='{{endpoint "users"}}'
           hx-trigger="load"
           hx-swap="innerHTML">
    <p>Loading users…</p>
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
      <button type="button"
              hx-get='{{endpoint "users" .ID}}'
              hx-target="#user-detail"
              hx-swap="innerHTML">{{.Name}}</button>
    </li>
  {{else}}
    <li>No users found.</li>
  {{end}}
</ul>
{{end}}

{{define "users:detail"}}
  <article>
    <h2>{{.Name}}</h2>
  </article>
{{end}}
```

The shell does not query the resource. It only renders URLs. The subsequent requests perform the resource operations:

```text
GET /admin/
  → render the directory shell
  → endpoint "users" becomes /admin/resources/users

GET /admin/resources/users
  → call Index with this request's context
  → consume the iterator and check its errors
  → render users:list with []User as dot

GET /admin/resources/users/u-42
  → parse the ID and call Show with this request's context
  → render users:detail with User as dot, or return 404
```

The `/resources/<name>` routing convention is illustrative. Templates refer to names so that changing this convention need not change the markup.

### Direct CRUD wiring, without a required DTO mapper

All three candidates can use this resource descriptor:

```go
func readOnlyUsers(repo crud.Finder[User, UserID]) htmx.ResourceHandler[User, UserID] {
    users := htmx.ResourceHandlerFromCRUD[User, UserID](repo)

    // The dynamic repository type might expose more than the Finder interface.
    // Deliberately keep this resource read-only.
    users.Create = nil
    users.Update = nil
    users.Destroy = nil

    users.Views = userFragments()
    return users
}
```

This binds `FindAll` to `Index` and `FindByID` to `Show`. It does not expose `DeleteAll` or `Purge`, generate an admin UI, or require copying entities into a framework-specific envelope. The resource adapter handles iterator errors before rendering; list results must still be bounded or paginated.

A repository suitable for this direct path must already be appropriate for the audience, including any required tenant scoping. The constructor does not make an unrestricted repository safe for every authenticated user.

## A. Register, then mount

**API style:** mutable configuration during startup, ordinary structs for operations, error-returning registration methods, one bound mount per app.

This is the closest continuation of the current draft.

### A1. A directory backed directly by CRUD ports

```go
func MountDirectory(router *httpkit.Router, repo crud.Finder[User, UserID]) error {
    app := &htmx.App{
        Templates: directoryTemplates(),
    }

    if err := app.Resource("users", readOnlyUsers(repo)); err != nil {
        return err
    }
    if err := app.Page("/", "directory", "page"); err != nil {
        return err
    }

    return app.Mount(router, "/admin")
}
```

`App.Page` selects the app-relative route, template namespace and full-document definition. No page data is needed for this particular shell.

The useful properties of this style are:

- The operations remain ordinary function fields, like `httpkit.RESTHandler`.
- `App.Resource` associates a stable name with the descriptor; it need not immediately render or parse anything.
- `Mount` supplies the base, installs the mount-bound template helpers, validates view bindings and publishes the prepared handler.
- Registration mistakes can be reported immediately; preparation errors return from `Mount`.
- The mounted app is frozen. A second mount or late registration is an error in this candidate, not an instruction to rewrite existing URLs.

### A2. The same UI, backed by a current-user-aware layer

The application's operations can have the same shapes without implementing a CRUD repository:

```go
// An application-owned role, not an interface required by the htmx package.
type VisibleUserDirectory interface {
    ListVisibleUsers(context.Context) iter.Seq2[User, error]
    FindVisibleUser(context.Context, UserID) (User, bool, error)
}

func visibleUsers(service VisibleUserDirectory) htmx.ResourceHandler[User, UserID] {
    return htmx.ResourceHandler[User, UserID]{
        Index: service.ListVisibleUsers,
        Show:  service.FindVisibleUser,
        Views: userFragments(),
    }
}

func MountPersonalDirectory(router *httpkit.Router, service VisibleUserDirectory) error {
    app := &htmx.App{
        Templates: directoryTemplates(),
    }

    if err := app.Resource("users", visibleUsers(service)); err != nil {
        return err
    }
    if err := app.Page("/", "directory", "page"); err != nil {
        return err
    }

    return app.Mount(router, "/account")
}
```

**The HTML is unchanged.** Its collection URL is now `/account/resources/users`.

The application service reads the authenticated principal from the request context and applies its visibility rules to both listing and individual lookups. Its methods may perform scoped queries, combine data, or invoke other use cases. A scoped object that *does* implement CRUD ports can instead use `readOnlyUsers` directly.

It is also possible to start with `ResourceHandlerFromCRUD` and replace both `Index` and `Show`. Replacing only the list operation would not protect a direct item request. Future writes must enforce their own scope too.

### A3. Mounting below an existing namespace

For the same app configuration, **before its first mount**, the final mounting step could instead be:

```go
app.MountPoint = "/tools/admin"
return app.Mount(router.Sub("/tools"), "/admin")
```

Here `/admin` is the path local to that mux, while `/tools/admin` is the complete browser-visible app base. The same template helper then returns `/tools/admin/resources/users`.

This explicit override is useful because an arbitrary `httpkit.Multiplexer` cannot reveal all of its enclosing prefixes. It is not a second prefix to append to the mount path. Reverse-proxy prefixes likewise come from trusted configuration, not client HX headers.

### Trade-off

**Best when:** the desired API is a small extension of existing httpkit conventions.

**Cost:** repeated error checks during registration, and a startup-only mutable lifecycle that needs clear rules. The string names in templates are checked at setup/render time rather than by the Go compiler.

## B. Declare the app in one value

**API style:** an app literal contains page and resource registrations; mounting validates and prepares the whole declaration.

This candidate introduces two illustrative descriptor names:

- `ResourceRegistration`: a logical name plus a resource descriptor.
- `PageRegistration`: an app-relative path plus a template selection.

They are not additions committed to the main draft. In particular, `Handler` below means an HTML resource descriptor that can be bound at mount time, not an arbitrary JSON REST handler.

### B1. Declarative composition with the same direct CRUD resource

```go
func MountDirectory(router *httpkit.Router, repo crud.Finder[User, UserID]) error {
    app := htmx.App{
        Templates: directoryTemplates(),
        Resources: []htmx.ResourceRegistration{
            {
                Name:    "users",
                Handler: readOnlyUsers(repo),
            },
        },
        Pages: []htmx.PageRegistration{
            {
                Path:      "/",
                Namespace: "directory",
                Template:  "page",
            },
        },
    }

    return app.Mount(router, "/admin")
}
```

To use the application layer from A2, replace only the resource entry:

```go
{
    Name:    "users",
    Handler: visibleUsers(service),
}
```

That last block is a Go composite-literal **excerpt**, intended inside the `Resources` slice, not a standalone declaration.

The template files and `endpoint` calls stay the same. A larger app can collect registrations from several UI packages before mounting; those packages do not need to agree on a deployment prefix.

### Intended lifecycle

- Build a complete declaration from ordinary Go values.
- `Mount` checks duplicate names, route collisions, enabled operations, view bindings and the public base together.
- Preparation produces a private snapshot; callers do not mutate the supplied maps, slices or descriptors while it is being prepared.
- Serve only after preparation succeeds. No partially usable app should appear because the last registration was invalid.
- For this candidate, keep one mount per configured app. A separate instance is used for a different prefix.

A slice, rather than a map keyed by name, makes duplicate declarations diagnosable and preserves declaration order. The registry still needs some narrow common binding interface so differently typed `ResourceHandler[ENT, ID]` values can coexist; the exact public interface is intentionally not designed here.

### Trade-off

**Best when:** the whole UI is assembled in one composition root or from configuration fragments.

**Cost:** more descriptor types, errors collected later at mount time, and less direct correspondence with `router.Resource(...)` calls. There should be no reflection-based dependency injection or hidden discovery behind the declaration.

## C. Register typed references, then create mounted instances

**API style:** registration returns an opaque resource reference, while mounting returns a distinct immutable mounted instance.

This candidate explores two open questions in the main draft:

1. Could Go code refer to resources without repeating names or losing ID types?
2. Could the same app definition be mounted twice without changing the first mount's generated URLs?

Illustrative additions for this candidate only:

- `htmx.RegisterResource[ENT, ID]`: a generic **package function**, not a generic method on `App`.
- `htmx.ResourceRef[ID]`: a registration identity with an ID type, not a URL and not a repository accessor.
- `Mount` returns a `*htmx.MountedApp` plus an error, unlike A/B.
- `htmx.Endpoint(mounted, ref, id...)`: a generic Go-side URL helper whose ID type comes from the reference.
- `App.Page` accepts explicit page data in this example. That is another tentative call shape, not an existing overload.

### C1. Registration, page links and two mounts

```go
type DirectoryLinks struct {
    Users htmx.ResourceRef[UserID]
}

func MountDirectoryPreviews(router *httpkit.Router, repo crud.Finder[User, UserID]) error {
    app := &htmx.App{
        Templates: directoryTemplates(),
    }

    users, err := htmx.RegisterResource[User, UserID](app, "users", readOnlyUsers(repo))
    if err != nil {
        return err
    }
    if err := app.Page("/", "directory", "page", DirectoryLinks{Users: users}); err != nil {
        return err
    }

    admin, err := app.Mount(router, "/admin")
    if err != nil {
        return err
    }
    preview, err := app.Mount(router, "/preview")
    if err != nil {
        return err
    }

    adminURL, err := htmx.Endpoint(admin, users, UserID("u-42"))
    if err != nil {
        return err
    }
    previewURL, err := htmx.Endpoint(preview, users, UserID("u-42"))
    if err != nil {
        return err
    }

    fmt.Println(adminURL)   // /admin/resources/users/u-42
    fmt.Println(previewURL) // /preview/resources/users/u-42
    return nil
}
```

The shared shell can keep using `{{endpoint "users"}}`. If it should use the reference supplied by Go instead, replace the shell's collection attribute with:

```gotemplate
hx-get='{{endpoint .Users}}'
```

Only this shell's dot is `DirectoryLinks`; resource templates still receive `[]User` or `User`. The list's `{{endpoint "users" .ID}}` remains valid. No mandatory reference envelope is added to domain entities.

The mount-bound template helper accepts either a registered name or a reference. A Go helper can enforce the reference's `UserID` type at compile time; Go templates are still dynamically checked. Passing a handle belonging to an unrelated app is an error, not a way to resolve another app's routes.

### Intended lifetime model

```text
app definition
  ├─ named resources, operations and templates
  ├─ mounted instance: /admin
  │    └─ immutable endpoint/template bindings for /admin
  └─ mounted instance: /preview
       └─ immutable endpoint/template bindings for /preview
```

The first mount freezes the definition against further registrations. Later mounts can prepare additional independent bindings from that frozen definition. Neither mount mutates a shared "current base path", and URL lookup without a mounted instance fails.

Shared operation functions may still refer to the same repository/service; mounting twice does not duplicate storage or create independent business state. Authentication, authorisation and transaction context still arrive with each request.

A public-prefix override would belong to each mount's trusted configuration in this style, rather than a mutable app-wide field that one mount changes for the other. The exact mount-options type is left open.

### Trade-off

**Best when:** callers need typed Go-side links, independently packaged UI components, or multiple mounts of one definition.

**Cost:** an extra identity type, generic registration/URL functions and a two-stage lifetime to learn. Handles do not replace runtime validation, authorisation or the string-based template helper. This may be more machinery than the first release needs.

## A deliberate comparison: what changes and what does not

| Concern | A: methods | B: declarations | C: references + mounted instances |
| --- | --- | --- | --- |
| Supply a CRUD resource | `ResourceHandlerFromCRUD` | Same | Same |
| Supply custom scoped operations | `ResourceHandler{Index, Show, …}` | Same | Same |
| Register it | `app.Resource(name, handler)` | `Resources: []ResourceRegistration{…}` | `RegisterResource(app, name, handler)` |
| Reference it in HTML | `endpoint "users"` | Same | Same, optionally `endpoint .Users` |
| Choose the base | At `Mount` | At `Mount` | For each mounted instance |
| Mount more than once | Use separate apps | Use separate apps | Supported through separate immutable bindings |
| Find configuration errors | Registration and mount | Mostly mount | Registration and mount |
| Generate typed URLs in Go | Not part of this minimal candidate | Not part of this minimal candidate | Reference plus mounted instance |

The choices can later be combined, but doing so immediately would hide which additional API actually earns its place. In particular, these examples do **not** recommend implementing methods, declaration lists, fluent builders and typed handles all at once.

## Behaviour assumed by every candidate

The code above intentionally focuses on API ergonomics. It still relies on the following requirements from the main draft:

- **No eager data access from templates.** `endpoint` only resolves registration metadata. Every later HTTP request performs its own operation and access checks.
- **No hard-coded deployment prefix in HTML.** Resource and script helpers share the same public mount base. Unknown names, invalid IDs and unbound lookups fail rather than producing guessed URLs.
- **ID handling is owned by the resource binding.** Collection calls take no ID; item calls format a raw typed ID and path-escape it once. HTTP parsing must agree with URL generation.
- **Read-only really means read-only.** Inferring an implementation's CRUD capabilities is not approval to expose all of them. Missing operations are unavailable; bulk deletion is never inferred automatically.
- **Scope belongs in every relevant operation.** Protect individual item lookups and future mutations, not only the list query. Apply authentication and CSRF middleware to resource endpoints as well as page routes. Personalised responses need an appropriate private/no-store caching policy.
- **Errors precede commitment where possible.** Iterator, mapping and template failures flow through the common HTML error handler before writing a success body. `Show` with `found == false` becomes 404.
- **Template sets stay immutable while serving.** Binding a mount does not mean mutating shared template funcs on each request.
- **Async does not imply progressive enhancement.** The example shell depends on JavaScript to fetch its content. A no-JS experience needs real full-page destinations or initial server-rendered content. Fragment-only endpoints must not be used as if they were history-navigation pages.

### Why these examples do not enable writes yet

Writes are within the resource concept, but their view/input hooks are still open in the main draft. Merely adding `Update: service.Update` would leave important questions unanswered:

1. Which submitted fields are writable, and how are they decoded and validated?
2. How is the path-selected ID kept authoritative over any body ID?
3. How is the authenticated user's scope enforced for that mutation?
4. What is returned after success: an item fragment, a list, a redirect or another explicit response?
5. How are submitted values redisplayed on validation failure without persisting them?

A later write-oriented example should make those decisions visible before adding more configuration methods. It should reuse `Response`/`View`, form decoding and the error conventions already proposed, not invent a separate CRUD UI framework. The read examples above do not imply that htmx is limited to GET requests.

## Suggested starting point for discussion

**A is the smallest fit for the stated requirements:** it resembles `httpkit.RESTHandler`, supports direct CRUD wiring and custom application operations, and makes mount-bound template URLs easy to explain.

Choose **B** if a single declarative composition is more pleasant at real call sites. Choose **C** if typed references or repeatable mounts are actual requirements rather than hypothetical future flexibility.

A useful review exercise is to compare A1 and A2 first: can the same templates move from a direct resource to a user-scoped application layer, and from `/admin` to `/account`, without learning a new rendering model or rewriting their endpoint strings? All three candidates are intended to preserve that property.
