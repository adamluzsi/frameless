---
name: frameless-pathkit
description: Build, normalise and parse slash-separated URL and HTTP request paths with go.llib.dev/frameless/pkg/pathkit (Join, Clean, Canonical, Split, Unshift, SplitBase) instead of fmt.Sprintf or + concatenation, strings.Split/TrimPrefix/TrimSuffix slash juggling, or path.Join on URLs. Use when joining a base URL with path segments in an HTTP client, putting IDs or other dynamic values into request paths (escaping, path traversal), splitting a request path into segments or consuming it one segment at a time in a handler, normalising route patterns, mount points or configured path prefixes (leading, trailing and double slashes, dot segments), separating a base URL from its path, testing path handling, or changing pkg/pathkit itself. Not for filesystem paths. Applies to any Go project that imports go.llib.dev/frameless.
compatibility: Go projects importing go.llib.dev/frameless/pkg/pathkit.
license: MIT (matches go.llib.dev/frameless)
metadata:
  project: go.llib.dev/frameless
  package: pkg/pathkit
  scope: Global
---

# Frameless pathkit

`go.llib.dev/frameless/pkg/pathkit` handles slash-separated **URL and HTTP request paths**. Every result is rooted (`/…`), has no empty segments and has dot segments resolved, so you never have to think about missing, doubled or trailing slashes. `pkg/httpkit` (Router, RESTHandler, RESTClient, Mount) is built on it.

It is **not** for filesystem paths. For those, use `path/filepath`, and `os.OpenRoot` to confine access to a directory. `io/fs` names must not start with `/`, so use `path` and `fs.ValidPath` for them.

## Stop signs

If you are about to write any of these, use `pathkit` instead:

- `fmt.Sprintf("%s/users/%s", baseURL, id)`, or `baseURL + "/" + id`
- `strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(p, "/")`, or other slash juggling
- `path.Join(baseURL, …)`: it collapses `https://` into `https:/`
- `strings.Split(r.URL.Path, "/")` followed by skipping empty parts or indexing `parts[1]`
- hand-written "pop the first segment and keep the rest" logic in a handler
- `if !strings.HasPrefix(p, "/") { p = "/" + p }` to normalise route patterns or configured prefixes

Writing a router or REST endpoint by hand? Check `pkg/httpkit` first. `httpkit.Router` supports `:name` placeholders, and `httpkit.PathParams(r.Context())` returns their values already unescaped. `httpkit.RESTHandler` and `httpkit.RESTClient` cover REST resources.

## API

All examples below are verified outputs.

| Function | Result | Examples |
| --- | --- | --- |
| `Clean(p)` | rooted, cleaned, **no** trailing slash | `""` → `/` · `a/b/` → `/a/b` · `a//b` → `/a/b` · `../../etc/passwd` → `/etc/passwd` · `/a%2F..%2Fb` → unchanged |
| `Canonical(p)` | like `Clean`, but **keeps** one trailing slash | `a/b/` → `/a/b/` · `//a//` → `/a/` · `/a/./b/../c/` → `/a/c/` · `""` → `/` |
| `Split(p)` | cleaned segments, no empty strings | `/a/b/` → `[a b]` · `""` and `/` → `[]string{}` (non-nil) · `/a/../b` → `[b]` · `/a%2Fb/c` → `[a%2Fb c]` |
| `Unshift(p)` | first segment, rooted remainder | `/a/b/c` → `a`, `/b/c` · `a` → `a`, `/` · `/a/b/` → `a`, `/b/` · `/` → `""`, `/` · `""` → `""`, `""` |
| `Join(ps...)` | rooted path, or a URL when the first element has a scheme | `a`, `b` → `/a/b` · `/a/`, `b/` → `/a/b` · `https://x.io/api`, `users`, `1` → `https://x.io/api/users/1` · no args → `/` |
| `SplitBase(uri)` | `scheme://userinfo@host:port` and the rest | `https://u:p@x.io:8443/a?k=v#f` → `https://u:p@x.io:8443`, `/a?k=v#f` · `/a/b` → `""`, `/a/b` · `https://x.io` → `https://x.io`, `""` |

`Clean` vs `Canonical`: use `Clean` for comparisons, map keys and mount points. Use `Canonical` when a trailing slash means something, such as a "subtree" route or a redirect target.

`Split` is not URL-aware: `/a/b?k=v` → `[a b?k=v]`. Strip the query before splitting.

## The escaping contract

pathkit works on **escaped** paths. Only a literal `/` separates segments; `%2F` is data, and `Clean`, `Split` and `Unshift` leave escapes untouched.

- **Parsing a request:** start from `r.URL.EscapedPath()`, not `r.URL.Path`. The decoded `Path` turns an ID like `a%2Fb` into two segments.
- **After `Split` or `Unshift`:** run `url.PathUnescape` on each segment you use as a value. Reply `400` if it fails.
- **Building a path:** pass every dynamic value (IDs, names, user input) through `url.PathEscape` before `Join`. Unescaped values silently produce the wrong URL:

  | Call | Result |
  | --- | --- |
  | `Join("/foo", "a/b")` | `/foo/a/b` (an extra segment) |
  | `Join("/foo", "100%")` | `/` (the whole path is lost) |
  | `Join("/foo", "a?b=c")` | `/foo/a` (the rest is dropped) |
  | `Join("/foo", url.PathEscape("a/b"))` | `/foo/a%2Fb` ✓ |
  | `Join("/foo", url.PathEscape("100%"))` | `/foo/100%25` ✓ |

- Literal route parts such as `"users"`, `"/notes/"` or `":id"` placeholders don't need escaping.

## `Join` rules

- **The base URL needs a scheme.** Only the first element is checked for one. `Join("localhost:8080", "api")` returns `localhost:8080` (parsed as scheme `localhost`, with the path dropped). Write `http://localhost:8080`. `SplitBase` has the same limitation: `localhost:8080/api` → `localhost:`, `8080/api`.
- **The base keeps its scheme, userinfo, host, port and query.** `Join("https://x.io/api?k=v", "users")` → `https://x.io/api/users?k=v`.
- **Trailing slashes are dropped.** `Join("https://x.io/api/", "/users/")` → `https://x.io/api/users`. Append `"/"` yourself if an endpoint needs one. (`url.JoinPath` keeps the slash and returns an error; `pathkit.Join` does neither.) A bare host is the exception: `Join("https://x.io")` → `https://x.io/`.
- **Empty and `"/"` elements are ignored.** An empty ID therefore silently targets the collection: `Join(base, "users", "")` is `…/users`, so a `DELETE` would hit the collection endpoint. Reject empty IDs before joining.
- **Dot segments are resolved across all elements, including the base.** `Join("https://x.io/api", "../admin")` → `https://x.io/admin`.
- **A leading `//` on the first element is preserved.** `Join("//", "foo", "bar")` → `//foo/bar`. RFC 3986 treats this as a network-path reference: `url.Parse("//foo/bar")` gives `Host == "foo"`. Never let untrusted input become the first element.
- **No queries or fragments in elements.** Build the path with `Join`, then set the query with `url.Values`.

## Untrusted segments and path traversal

`url.PathEscape("..")` is still `..`, so escaping alone does not stop traversal:

```go
pathkit.Join("/files", url.PathEscape(".."), "secret") // "/secret"
```

`Clean` and `Join` never climb above `/`, but they can climb out of **your** prefix. Validate segments that come from outside before joining:

```go
func pathSegment(s string) (string, error) {
	switch s {
	case "", ".", "..":
		return "", fmt.Errorf("invalid path segment: %q", s)
	}
	return url.PathEscape(s), nil
}
```

## Patterns

### HTTP client: build request URLs

```go
type NoteClient struct {
	// BaseURL is the API root including the scheme, e.g. "https://api.example.com/v1".
	BaseURL    string
	HTTPClient *http.Client
}

func (c NoteClient) Get(ctx context.Context, userID, noteID string) (*http.Response, error) {
	u := pathkit.Join(c.BaseURL, "users", url.PathEscape(userID), "notes", url.PathEscape(noteID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return c.HTTPClient.Do(req)
}
```

With query parameters:

```go
u, err := url.Parse(pathkit.Join(c.BaseURL, "notes"))
if err != nil {
	return nil, err
}
q := u.Query() // keeps any query already present on BaseURL
q.Set("search", term)
u.RawQuery = q.Encode()
```

For a REST resource, prefer `httpkit.RESTClient`. Its `BaseURL` may contain `:name` placeholders that are filled from `httpkit.WithPathParam(ctx, name, value)`.

### HTTP handler: consume the path segment by segment

```go
func (h NoteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rawID, rest := pathkit.Unshift(pathkit.Canonical(r.URL.EscapedPath()))
	if rawID == "" { // "/"
		h.list(w, r)
		return
	}
	id, err := url.PathUnescape(rawID)
	if err != nil {
		http.Error(w, "malformed id", http.StatusBadRequest)
		return
	}
	switch pathkit.Clean(rest) { // rest keeps a trailing slash: "/attachments/"
	case "/":
		h.show(w, r, id)
	case "/attachments":
		h.attachments(w, r, id)
	default:
		http.NotFound(w, r)
	}
}
```

- Always `Canonical` (or `Clean`) the input before `Unshift`. `Unshift` cleans the path to find the first segment but returns the remainder **un-normalised**: `Unshift("/ab/../b/c")` → `b`, `/../b/c`.
- This works the same behind `http.StripPrefix` and `httpkit.Mount`, because both rewrite `RawPath` too.

### Normalise configured prefixes and patterns once

Normalise user-supplied base paths, mount points and route patterns where they enter the system, usually the constructor or config loading, rather than on every request:

```go
prefix := pathkit.Clean(cfg.PathPrefix) // "api/", "/api", "//api/" → "/api"
```

`httpkit.Mount(mux, pattern, h)` already applies `Clean` to `pattern`.

### Fill a path template

Use `SplitBase`, then `Split`, then `Join`. `httpkit.RESTClient` uses the same approach:

```go
base, p := pathkit.SplitBase(tmpl) // "https://x.io/users/:id/notes" → "https://x.io", "/users/:id/notes"
parts := []string{base}            // base is "" for a plain path; Join handles that
for _, seg := range pathkit.Split(p) {
	if name, ok := strings.CutPrefix(seg, ":"); ok {
		v, ok := params[name]
		if !ok {
			return "", fmt.Errorf("missing path param: %s", name)
		}
		seg = url.PathEscape(v)
	}
	parts = append(parts, seg)
}
return pathkit.Join(parts...), nil
```

`SplitBase` keeps the query and fragment in the path part. Remove them first if a template can contain them.

## Testing path handling

- Pin exact output strings. Cover these inputs: empty, `/`, no leading slash, trailing slash, double slashes, `.` and `..`, an escaped `%2F`, a literal `%`, spaces and non-ASCII characters, and a base URL with and without a path and a query.
- In HTTP handler tests, build targets the way the httpkit tests do: `httptest.NewRequest(http.MethodGet, pathkit.Join("/", url.PathEscape(id), "notes"), nil)`. Use an `id` containing `/` or `%` to prove the escaping round-trips.
- Follow the project's `go.llib.dev/testcase` spec style. `pkg/pathkit/pathkit_test.go` is a good template.

## Changing `pkg/pathkit` itself

- Keep it standard-library only (`net/url`, `path`, `regexp`, `strings`) and free of state.
- Its behaviour is load-bearing for `pkg/httpkit` (`Router`, routing context, `RESTHandler`, `RESTClient`, `Mount`) and for `adapter/hashicorp/vault`. Changing an expected output in `pathkit_test.go` is a breaking change for these dependants. Run:

  ```sh
  go test ./pkg/pathkit/... ./pkg/httpkit/... ./adapter/hashicorp/vault/...
  ```

  The Vault tests skip themselves unless a Vault instance is configured through `VAULT_*` env vars.
- Add a test for every edge case you touch. Add doc comments to exported functions that lack them (currently `Unshift`, `Canonical`, `Clean` and `Split`).
- Known quirks. Don't build on them, and pin any fix with a test:
  - `Unshift` finds the remainder with `strings.Index` on the raw input, so the remainder is not normalised.
  - `Join` detects a base URL with `^[^:]+:` on the first element, so `host:port` and `name:verb` look like URLs: `Join("users:batch")` → `users:batch`, but `Join("users")` → `/users`.
  - `Join`'s doc comment says a leading `//` resolves against the current URL path. RFC 3986 (and `url.Parse`) treat it as a host instead.
- Update the `pathkit` entry in `pkg/README.md` if the scope changes.

## Reference

- `pkg/pathkit/pathkit.go`: the whole package
- `pkg/pathkit/pathkit_test.go`: specs that pin current behaviour
- `pkg/httpkit/router.go`, `pkg/httpkit/internal/routes.go`, `pkg/httpkit/resthandler.go`, `pkg/httpkit/restclient.go`: real-world usage

## Checklist

- [ ] No `fmt.Sprintf`, `+`, `path.Join` or manual slash trimming to build URL or request paths.
- [ ] Every dynamic segment is passed through `url.PathEscape` before `Join`. Untrusted segments are also checked for `""`, `.` and `..`.
- [ ] Base URLs include a scheme, and untrusted input is never the first argument to `Join`.
- [ ] Queries are set with `url.Values` after `Join`, never inside path segments.
- [ ] Request paths are parsed from `r.URL.EscapedPath()`, passed through `Canonical` before `Unshift`, and each extracted segment is unescaped with `url.PathUnescape`.
- [ ] Configured prefixes and patterns are normalised once, at the boundary.
- [ ] Filesystem paths use `path/filepath` or `os.Root`, not `pathkit`.
