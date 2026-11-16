# Existing Go htmx helper libraries: APIs, patterns and gaps

Adoption numbers (stars, importers, last push) are in [landscape.md §2](landscape.md#2-htmx-and-hypermedia-helper-libraries-for-go). This file compares **API shapes**, so a frameless adapter can reuse what works and avoid known traps.

API details come from each project's README or source on its default branch (fetched 2026-11-16 via raw.githubusercontent.com). Source links are listed per library.

## 1. Summary matrix

| Capability | [donseba/go-htmx](https://github.com/donseba/go-htmx) | [angelofallars/htmx-go](https://github.com/angelofallars/htmx-go) | [pagoda `pkg/htmx`](https://github.com/mikestefanello/pagoda/blob/main/pkg/htmx/htmx.go) | [gomponents-htmx](https://github.com/maragudk/gomponents-htmx) | [donseba/go-partial](https://github.com/donseba/go-partial) |
|---|---|---|---|---|---|
| Request checks | `h.IsHxRequest()`, `IsHxBoosted()`, `IsHxHistoryRestoreRequest()`, `RenderPartial()` | `htmx.IsHTMX(r)`, `htmx.IsBoosted(r)` | `GetRequest(ctx) *Request` (struct, cached per request) | `hxhttp.IsBoosted(r.Header)` etc. (funcs over `http.Header`) | via connectors |
| Response headers | methods on a per-request `Handler` (`PushURL`, `ReTarget`, `ReSwapWithObject`, `TriggerWithObject`, …) | fluent `NewResponse().Reswap(…).Retarget(…).AddTrigger(…).Write(w)` | struct `Response{PushURL, Redirect, Trigger, …}.Apply(ctx)` | `hxhttp.SetPushURL(w.Header(), …)` etc. | helpers |
| Trigger JSON building | `NewTrigger().AddEvent().AddEventDetailed()` + notification helpers (`TriggerSuccess`, `TriggerError`, …) | `Trigger`, `TriggerDetail`, `TriggerObject` | raw string | n/a | n/a |
| Swap modifiers | `NewSwap()…`, `StringFor(version)` | `SwapInnerHTML.ShowOn(...)…` (v2 syntax) | n/a | attributes | n/a |
| Template integration | components over `html/template` | `RenderTempl(ctx, w, c)` | gomponents (app-level) | gomponents | **its own partial tree over `html/template`** |
| **htmx 4** | **opt-in** `SetVersion(htmx.Version4)` on the instance; package-level funcs and middleware stay v2 | **no** (last push 2024-11) | **no** (still sends `HX-Trigger-After-*`) | not verified | "HTMX 4" + "protocol audit" sections |
| SSE | yes (package) | no | no | no | yes |
| Router coupling | `net/http` | `net/http` | **Echo** | `net/http` | connectors |

## 2. Library notes

### donseba/go-htmx: the most complete and the only maintained one with htmx 4

- Per-request handle: `h := app.htmx.NewHandler(w, r)`. It has request checks, response header setters, and `h.Write`/`JustWriteString`.
- **htmx 4 is opt-in per instance** (`app.htmx.SetVersion(htmx.Version4)`). The README has a version table:
  - `RenderPartial()` becomes false for `HX-Request-Type: full`.
  - `TargetID()`/`SourceID()` decode `tag#id`.
  - `Prompt()` is URI-decoded.
  - `ReSwapWithObject` renders v4 swap syntax (`show:top showTarget:"#list" focusScroll:true`).
  - `TriggerAfterSwap*`/`TriggerAfterSettle*` send nothing.
  - `StopPolling()` still sends 286, which v4 "swaps like a 200 while it keeps polling".
- **Trap:** the package-level `htmx.IsHxRequest(r)`, `htmx.RenderPartial(r)` and the middleware "have no version: they keep the htmx 2 behaviour". It is easy to get wrong when mixing styles.
- Version is a **server-side setting**, not detected per request. *Inference:* per-request detection from the presence of `HX-Request-Type`/`HX-Source` would make mixed or rolling deployments safer.
- Small adoption: **19 importers**.

### angelofallars/htmx-go: the nicest response builder, now stale

- `htmx.NewResponse().Retarget("#x").Reswap(htmx.SwapBeforeEnd.Scroll(htmx.Bottom)).AddTrigger(htmx.TriggerObject("e", v)).Write(w)`
- `RenderTempl(ctx, w, component)` writes headers, then renders the templ component.
- The README warns: *"If you're adding additional headers to a global response writer, always use the `.Clone()` method"*. Its mutable builder is unsafe to share, which is a design smell to avoid.
- `htmx.StatusStopPolling` (286) and `AddTriggerAfterSwap/Settle` are **htmx-2 only**.
- Most-starred (807) but **stale since 2024-11**, with 41 importers.

### pagoda `pkg/htmx`: the "write it yourself in 80 lines" baseline

- `Request{Enabled, Boosted, HistoryRestore, Trigger, TriggerName, Target, Prompt}` is parsed once and cached on the Echo context.
- `Response{PushURL, Redirect, Refresh, ReplaceURL, Trigger, TriggerAfterSwap, TriggerAfterSettle, NoContent}.Apply(ctx)`.
- The UI layer gets `Request.Htmx`, and `Render()` automatically returns only the page content for non-boosted htmx requests.
- This is the pattern most apps hand-roll. It's simple and readable, but htmx-2 only and router-coupled.

### gomponents-htmx: attribute builders + header funcs

- Attributes such as `hx.Boost("true")`, `hx.Target("#partial")` and `hx.Swap("outerHTML")` are type-safe builders for gomponents.
- `hxhttp` exposes **free functions over `http.Header`**: `IsBoosted(r.Header)`, `SetPushURL(w.Header(), url)`. They are minimal, have no state, and are easy to test.
- gomponents' `http.Adapt(func(w, r) (Node, error))` is a **handler adapter that returns a renderable value or an error**. That shape maps well to frameless error handling (see [adapter-design-implications.md](adapter-design-implications.md)).

### donseba/go-partial: the maximal approach

- It grew out of go-htmx. Its README has 63 sections, about 94 KB, covering:
  - A partial tree.
  - Typed "contracts" via `@model` comment annotations (go-clue).
  - `CheckTemplates()` startup validation, with file:line problems and "did you mean" hints.
  - OOB regions, slots and fragments.
  - SSE, CSP, CSRF, flash messages and localisation.
  - Metrics and diagnostic events.
  - An htmx 4 protocol audit.
- **Good ideas to borrow:** `CheckTemplates()` at startup (it reports undefined funcs, missing `{{template}}` names and parse errors with positions), and render-time target fallback.
- **Inference / judgement:** the breadth is the risk. One package owns templating, transport, security and UX concerns at once. 24 stars suggests the market hasn't adopted that scope. A frameless adapter should stay closer to go-htmx's surface and borrow go-partial's checks.

### cattlecloud/webtools `htmx`: multi-fragment responses

Mentioned on HN by a production user ([sethops1](https://news.ycombinator.com/item?id=48913966)):

```go
return htmx.Write(w,
    &htmx.Template{FS: htmx.FS(ui.HTMX, "parts"), Filename: "arrows.html", Fields: []any{thread, up}},
    &htmx.Component{HTML: `<div … hx-swap-oob=true id="points-{{$thread}}">{{$count}} points</div>`, Fields: []any{count, thread}},
)
```

It shows the demand for **"main fragment + N OOB fragments" in one call**. The `Fields: []any` + `index . 0` part shows the pain of passing multiple arguments to stdlib templates.

### templ: fragments and interop (not a helper library, but sets expectations)

- `templ.Component` is `Render(ctx context.Context, w io.Writer) error`.
- Fragments: mark regions with `@templ.Fragment("name") { … }` and render them with `templ.RenderFragments(ctx, w, component, "name")`, or `templ.Handler(c, templ.WithFragments("name"))`.
- Interop: `templ.FromGoHTML(t, data)` (stdlib → templ) and `templ.ToGoHTML(ctx, c)` (templ → `template.HTML`).

### datastar-go: the contrasting model

- `sse := datastar.NewSSE(w, r)`, then `sse.PatchElements(html)`, `sse.MarshalAndPatchSignals(v)` and `sse.Redirect(url)`. Incoming signals are read with `datastar.ReadSignals(r, &v)`.
- Every interaction is an SSE stream of DOM and signal patches, not request/response HTML swaps. It's out of scope for an htmx adapter. An SSE primitive built on frameless pub/sub could serve both later (inference).

## 3. Patterns compared

| Concern | Options seen | Assessment for frameless |
|---|---|---|
| Reading request headers | free funcs over `*http.Request` / `http.Header`; per-request handler object; parsed struct cached in ctx | **Parsed, immutable value struct**, available both as `htmx.RequestFrom(r)` (or similar) and via ctx from middleware. It's testable, version-normalised, and needs no object lifecycle. |
| Writing response headers | fluent mutable builder (needs `Clone()`); methods on handler; value struct `.Apply`; free header funcs | **Value struct with an `Apply(http.Header)`-style method**, or small free funcs. Value semantics avoid the shared-builder trap, and trigger merging can live in the type. |
| Version handling | none (v2 only); per-instance opt-in (go-htmx) | **Detect per request** (`HX-Request-Type`/`HX-Source` ⇒ v4), with an explicit override. Render version-specific syntax (`HX-Reswap` modifiers, trigger headers). |
| Rendering | library-owned partial trees (go-partial); templ-specific (`RenderTempl`); app-level | **Small `Component` interface + stdlib template set**; no tree, no DSL. |
| Validation | none; `CheckTemplates()` (go-partial) | Startup checks for the stdlib template set: parse all, execute page and fragment samples, report everything. |

## 4. Gaps a frameless adapter could fill

1. **htmx 4 as the default, with v2 compatibility per request.** No library does this. go-htmx comes closest but needs an opt-in setting per instance.
2. **Fragment and multi-target composition for stdlib templates.** OOB for v2, `<hx-partial>` for v4, plus `<title>` handling. Today people use `[]any` workarounds.
3. **HTML-aware error handling.** None of the helpers map domain errors to the right status *and* a swappable fragment. That matters more in v4, which swaps 4xx/5xx by default.
4. **Security defaults for htmx attributes in stdlib templates.** None of the surveyed helpers ship `json`/`path` funcs or document the `hx-on:*` escaping gap ([stdlib-html-template.md §5](stdlib-html-template.md#5-security-pitfalls-with-htmx-attributes)).
5. **Test support.** No library offers request builders with htmx headers (v2/v4) or response assertions. frameless can, through its testcase/`httptest` style.
6. **Integration with a router, errors and repositories.** Most libraries stop at headers. frameless already has `httpkit.Router`, `rfc7807`/`ErrorMapping`, `crud` ports, `formurlencoded` and `pathkit` to build on.
