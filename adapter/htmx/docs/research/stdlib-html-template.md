# Using stdlib `html/template` for dynamic (htmx) web pages

The user asked to lean toward stdlib templates, so this file covers what `html/template` does well and badly for htmx apps, and how experienced Go developers structure it.

Every behaviour marked **verified** was reproduced with small programs on **Go 1.27.3** (`go1.27.3 darwin/arm64`). Outputs are copied verbatim.

## 1. Why developers stay on stdlib

| Strength | Detail |
|---|---|
| Built-in and stable | No dependency, no codegen, no CLI, no editor plugin. Usage proxy: 118k importers vs templ's 7.6k ([landscape.md](landscape.md)). |
| Contextual auto-escaping | Escapes differently in HTML text, attributes, URLs, JS and CSS contexts. Users call this out as the main reason to stay ([community-sentiment.md §3.1](community-sentiment.md#31-stdlib-templates-are-good-enough-a-pragmatic-majority-voice)). |
| Fits the modern stdlib web stack | `embed` + `ParseFS` (Go 1.16). `ServeMux` method and wildcard patterns, and `http.FileServerFS` (Go 1.22). [Range-over-func and range-over-int in templates (Go 1.24)](https://go.dev/doc/go1.24). [`http.CrossOriginProtection`](https://go.dev/doc/go1.25) (Go 1.25). |
| Fragments are native | `{{block "name" .}}` declares a fragment inline, and `ExecuteTemplate(w, "name", data)` renders only that fragment. This is "template fragments", the htmx-recommended pattern. |
| No build step | Templates can be reloaded from disk in development (`os.DirFS`) and embedded in production (`embed.FS`). |

**Verified:** `{{block}}` fragments

```text
full page      <html><body><tr><td>Ada</td></tr><tr><td>Bob</td></tr></body></html>
fragment rows  <tr><td>Ada</td></tr><tr><td>Bob</td></tr>
```

**Verified:** `range` over an `iter.Seq2[T, error]` works. That is the exact shape of frameless `crud.AllFinder.FindAll`:

```gotemplate
{{range $u, $err := .}}{{if $err}}ERR{{else}}<li>{{$u.Name}}</li>{{end}}{{end}}
```

```text
<li>Ada</li><li>Bob</li>
```

An adapter could pass `FindAll(ctx)` straight to a template without collecting it into a slice first. That has two catches:

- If iteration fails part-way, the error is only visible as `$err` inside the template.
- Rendering is not side-effect free.

Both are arguments for buffered rendering (§4).

## 2. The canonical stdlib + htmx pattern (Alex Edwards, 2026)

Source: [How I use HTMX with Go](https://www.alexedwards.net/blog/how-i-use-htmx-with-go) (HN: 315 points). It targets **htmx 2.0.10**.

Layout:

```text
assets/
  html/
    base.tmpl          {{define "base"}} … {{template "page:title" .}} … {{template "page:content" .}}
    partials/*.tmpl    {{define "partial:…"}}
    pages/*.tmpl       {{define "page:title"}}…{{end}} {{define "page:content"}}…{{end}}
  static/js/htmx.min.js
```

The renderer, abbreviated from the article:

```go
type htmlRenderer struct {
    templateFS      fs.FS
    sharedTemplates *template.Template // base + partials, parsed once with a FuncMap
}

func (h *htmlRenderer) render(w http.ResponseWriter, status int, data any, templateName string, additionalTemplateFiles ...string) error {
    ts, err := h.sharedTemplates.Clone()          // per request
    // ...
    ts, err = ts.ParseFS(h.templateFS, additionalTemplateFiles...) // the page file
    buf := new(bytes.Buffer)
    err = ts.ExecuteTemplate(buf, templateName, data)               // "base" or a fragment name
    // ...
    w.WriteHeader(status)
    buf.WriteTo(w)
    return nil
}
```

The other helpers the article builds:

- **`isHTMXRequest(r)`** chooses the template name, for example the full `"base"` page vs a `"users:rows"` fragment for a search box.
- **`Vary: HX-Request`** is set on *every* response in `render()`. The article does it there so individual handlers can't forget it.
- **`redirect(w, r, url)`**: for htmx requests it sends `HX-Redirect` with a 2xx status. Otherwise it falls back to `http.Redirect`, which keeps the app working without JavaScript.
  - The article explains why it avoids `HX-Location`: the target handler can't tell that request apart from a normal htmx partial request.
- **`browserURL(r)`** parses `HX-Current-URL` and falls back to `r.URL`.
- **`<meta name="htmx-config">`** starting point:

  ```json
  {
    "includeIndicatorStyles": false,
    "historyCacheSize": 0,
    "historyRestoreAsHxRequest": false,
    "responseHandling": [
      {"code": "204", "swap": false},
      {"code": "422", "swap": true},
      {"code": "[45]..", "swap": true, "target": "body"},
      {"code": "...", "swap": true}
    ],
    "timeout": 5000
  }
  ```

  The article also recommends `disableInheritance: true`.
  - Several of these settings are htmx 2 workarounds that became htmx 4 **defaults**: explicit inheritance, no local history cache, and swapping 4xx/5xx. Others were renamed or removed.
  - See [htmx-4-server-contract.md](htmx-4-server-contract.md). An adapter targeting htmx 4 needs a different and smaller default config.
- **Page-specific layouts:** `{{template "layout:sidebar" .}}` from within the page.

## 3. Organising a template set

How to arrange layout, pages and partials is the most-debated stdlib topic ([community-sentiment.md §3.7](community-sentiment.md#37-the-clone-debate-stdlib-template-organisation-is-unclear)).

The underlying constraint: a template namespace has **one definition per name**. Every page wants to define `page:content`, so two pages can't share one namespace.

| Strategy | How | Pros | Cons |
|---|---|---|---|
| A. Clone per request (Alex Edwards) | `shared.Clone()` + `ParseFS(page)` on every render | Simple; pages reuse block names | Parses on every request; parse errors surface at request time |
| **B. Clone per page at startup** | For each page file: `shared.Clone()` → `ParseFS(page)` → store in `map[string]*template.Template` | Parse once; all parse errors at boot; still reuses block names | Must finish before any `Execute` on `shared` (see below) |
| C. One namespace, unique names | `users/index:content`, `users/show:content`, … | Single parse, no cloning | Layout must be parameterised by name; verbose |
| D. Base as begin/end halves | `{{template "base:begin"}}…{{template "base:end"}}` ([akazantsev](https://news.ycombinator.com/item?id=48917295)) | No clone | Titles and slots go through data; awkward for nested layouts |

**Verified:** cloning is only possible *before* the set has executed:

```text
html/template: cannot Clone "base" after it has executed
```

So strategy B must build every page clone eagerly at startup. Lazy cloning after the first render would fail. This is an easy rule for an adapter to enforce, and one users often trip over.

**Recommendation (proposal):** use strategy B with a filesystem convention, as the default in the adapter:

```text
layouts/*.tmpl   → shared (with partials), may be several named layouts
partials/*.tmpl  → shared
pages/**/*.tmpl  → one clone each, keyed by path ("users/index")
```

In development, re-run the same loader on each request (or on file change) from `os.DirFS`. In production, use `embed.FS`.

## 4. Failure modes at runtime

**Verified:**

```text
missing struct field   template: t:1:2: executing "t" at <.Nope>: can't evaluate field Nope in type main.User
missing map key        []                                  ← silently empty
missingkey=error       template: t:1:3: executing "t" at <.nope>: map has no entry for key "nope"
```

Implications for an adapter:

1. **Always buffer.** An execution error midway through a template would otherwise leave a half-written 200 response. Alex Edwards buffers too.
   - A `sync.Pool` of `bytes.Buffer` is the usual optimisation (inference: common practice, not measured here).
   - frameless `httpkit.TransactionMiddleware` buffers until commit as well, so the two compose.
2. **Default to `Option("missingkey=error")`.** This turns silent blanks into errors. It matters most when data is passed as `map[string]any`, which is common for multi-argument partials.
3. **Startup render checks (proposal):** let pages declare a sample data value, then execute every page and fragment once into `io.Discard` at boot or in a test. That catches the "`Titl` typo" class of bug ([thraxil](https://news.ycombinator.com/item?id=38601195)) without codegen.
   - For static checking, see [jba/templatecheck](https://github.com/jba/templatecheck), which is by a Go team member.
4. **Per-request funcs** (current user, CSRF token, locale) need a `Clone().Funcs(...)` per request today. A proposal to avoid that, [golang/go#54450 "add ExecuteFuncs"](https://github.com/golang/go/issues/54450), is **still open**. Two practical options:
   - Pass request-scoped values in a view-model wrapper (`{{.Request.CSRF}}`).
   - Define funcs that take the request/ctx as an argument (`{{asset .Ctx "app.css"}}`).

## 5. Security pitfalls with htmx attributes

This is the most important finding for an adapter that leans on stdlib. **html/template does not know htmx's attributes**, so it escapes them as plain HTML attribute values.

**Verified.** The untrusted input is `x');alert(1);('` placed in `f('{{.}}')`:

```text
onclick                <b onclick="f('x\u0027);alert(1);(\u0027')">              ← JS-escaped: safe
hx-on:click            <b hx-on:click="f('x&#39;);alert(1);(&#39;')">            ← HTML-escaped only
hx-on::after-request   <b hx-on::after-request="f('x&#39;);alert(1);(&#39;')">
hx-on-click            <b hx-on-click="f('x&#39;);alert(1);(&#39;')">
data-hx-on:click       <b data-hx-on:click="f('x&#39;);alert(1);(&#39;')">
alpine x-on:click      <b x-on:click="f('x&#39;);alert(1);(&#39;')">             ← same for Alpine
alpine @click          <b @click="f('x&#39;);alert(1);(&#39;')">
alpine x-data          <b x-data="{v:'x&#39;);alert(1);(&#39;'}">
```

`&#39;` is decoded back to `'` by the HTML parser before htmx or Alpine evaluates the attribute as JavaScript. **The result is an XSS vector**: `html/template` treats only `on*` attributes as JS context.

**Verified fix:** emit a JSON value with a `json` template func (`json.Marshal` → `string`). The output is a valid JS string literal in an attribute and valid JSON for `hx-vals`:

```text
hx-on:click + json   <b hx-on:click="f(&#34;x&#39;);alert(1);(&#39;&#34;)">   → f("x');alert(1);('")
hx-vals map          <b hx-vals='map[id:1 q:a&#34;b]'>                          ← Go syntax: broken
hx-vals json         <b hx-vals='{&#34;id&#34;:1,&#34;q&#34;:&#34;a\&#34;b&#34;}'> → {"id":1,"q":"a\"b"}
```

**URL-ish attributes are inconsistent. Verified:**

```text
href javascript:          <a href="#ZgotmplZ">                ← filtered (URL context)
hx-push-url javascript:   <a hx-push-url="#ZgotmplZ">         ← filtered (attribute name contains "url")
hx-get javascript:        <a hx-get="javascript:alert(1)">    ← NOT filtered (plain attribute)
href odd id               <a href="/users/a%20b?c#d/e">       ← normalised, but ? and # still split the URL
hx-get odd id             <a hx-get="/users/a b?c#d/e">       ← not even normalised
hx-get pathEscape         <a hx-get="/users/a%20b%3Fc%23d%2Fe">
hx-get pathEscape ..      <a hx-get="/users/..">              ← PathEscape leaves dot segments alone
```

`hx-get`, `hx-post`, `hx-put`, `hx-patch` and `hx-delete` are not URL context. Neither the scheme filter nor URL normalisation applies to them. Dynamic segments must be escaped explicitly.

Verified with frameless `pathkit` (from a throwaway test in `pkg/pathkit`, since deleted):

```text
"42"         Join="/users/42"     JoinEsc="/users/42"
".."         Join="/"             JoinEsc="/"
"a b?c#d/e"  Join="/users/a%20b"  JoinEsc="/users/a%20b%3Fc%23d%2Fe"
"../admin"   Join="/admin"        JoinEsc="/users/..%2Fadmin"
```

`pathkit.Join(base, url.PathEscape(id))` handles everything except IDs that are empty or literally `.` or `..`. The `frameless-pathkit` skill also says to reject `""`, `.` and `..` for untrusted segments.

**Recommended template funcs** (proposal):

| Func | Purpose |
|---|---|
| `json` / `hxvals` | Marshal to JSON for `hx-vals`, `hx-headers`, and values inside `hx-on:*` and Alpine attributes. |
| `path` | `pathkit.Join` with every dynamic segment `url.PathEscape`d. Rejects `""`, `.` and `..`, and returns a value that is safe for `hx-get`. |
| `asset` | Hashed URL for a mounted static file (see [adapter-design-implications.md](adapter-design-implications.md)). |
| `dict` | Build a `map[string]any` for multi-argument partials. Pair it with `missingkey=error`. This answers the most-cited complaint ([pagoda](https://github.com/mikestefanello/pagoda#why-gomponents), [markusw](https://news.ycombinator.com/item?id=41876471)). sprig's popularity shows the demand. |

Also document the rule: **never interpolate untrusted data into `hx-on:*`, `x-*` or `@*` without `json`**. A further option is an opt-in check at parse time that scans for `{{` inside `hx-on` attributes, to flag that pattern (inference: feasible with `text/template/parse`, not prototyped).

**CSRF:** the stdlib answer since Go 1.25 is `http.CrossOriginProtection`, which uses `Sec-Fetch-Site` and `Origin`. htmx requests are same-origin `fetch` calls, so they pass without token plumbing.

Token-based alternatives such as nosurf and gorilla/csrf need `hx-headers:inherited='{"X-CSRF-Token": …}'` on `<body>` in htmx 4. `hx-headers` without `:inherited` would not propagate, because inheritance is explicit in v4.

## 6. Fragments, OOB and multi-target responses

| Need | stdlib technique |
|---|---|
| Render only part of a page | `{{block "rows" .}}…{{end}}` in the page + `ExecuteTemplate(buf, "rows", data)` |
| Same handler serves full page or fragment | Pick the template name from the htmx request type. Must set `Vary`. |
| Update several regions at once (htmx 2 and 4) | Execute several fragments into one buffer; the extra ones carry `hx-swap-oob="true"` and an `id`. |
| Update several regions at once (htmx 4) | Wrap each fragment in `<hx-partial hx-target="#id" hx-swap="…">`. A response containing **only** `<hx-partial>` elements skips the main swap. |
| Change `<title>` with a fragment | Emit a top-level `<title>` next to the fragment ([yawaramin](https://news.ycombinator.com/item?id=48915935)). With stdlib this is easiest as a wrapper the adapter adds, not something each fragment template remembers. |

A multi-fragment builder is a natural adapter feature: `render fragment A as main, B and C as partials`. It removes the `[]any` + `index . 0` workaround seen in the wild ([sethops1](https://news.ycombinator.com/item?id=48913966)).

## 7. Interop with templ and gomponents

Users don't need to choose exclusively:

- **templ** components implement `Render(ctx context.Context, w io.Writer) error` (`templ.Component`).
  - `templ.FromGoHTML(t, data)` embeds an `html/template` in templ.
  - `templ.ToGoHTML(ctx, c)` turns a templ component into `template.HTML` for use inside stdlib templates.
  - templ also has fragments: `@templ.Fragment("name")` + `templ.RenderFragments`.
- **gomponents** nodes implement `Render(io.Writer) error`. The [htmx "template fragments" essay](https://htmx.org/essays/template-fragments/) lists both stdlib `{{block}}` and templ as Go implementations.

An adapter that renders a one-method `Component` interface, with the same signature as `templ.Component`, supports templ structurally and gomponents with a one-line adapter func. Neither becomes a dependency. See [adapter-design-implications.md](adapter-design-implications.md).

## 8. Summary: stdlib papercuts and how an adapter could address them

| Papercut | Source | Adapter answer (proposal) |
|---|---|---|
| Layout/page/partial organisation, the `Clone()` debate | Alex Edwards thread, pagoda | Filesystem convention + per-page clones at startup |
| Runtime-only errors | thraxil, pagoda | Buffering, `missingkey=error`, startup render checks, a test helper |
| Multi-argument components | pagoda, markusw, sethops1 | `dict` func; typed view models; `Component` interface for typed components |
| Missing funcs | sprig's 1.8k importers | Small curated FuncMap: `json`, `path`, `asset`, `dict` |
| htmx attribute XSS / URL gaps | Verified above | Safe funcs + documentation + optional lint |
| Fragment / OOB plumbing | yawaramin, sethops1 | Fragment selection by request type; multi-fragment response builder |
| Per-request funcs | golang/go#54450 | View-model wrapper with request data; no per-request clone |
| Editor support | lucideer, rickette | Recommend the `.go.html` (or `.gohtml`) suffix convention in docs |
