# htmx 4 server contract: what a Go adapter must handle

**htmx 4.0.0 was released on 2026-08-28** (4.0.1 on 2026-11-01). htmx 2.x is still maintained (2.0.11 on 2026-11-01). Most Go tutorials and helper libraries, including Alex Edwards' 2026 article (htmx 2.0.10), still target htmx 2. A new adapter should be **htmx-4-first and still serve htmx 2 clients**.

Sources:

- Docs: [four.htmx.org](https://four.htmx.org/), [`llms-full.txt`](https://four.htmx.org/llms-full.txt), and [What's new in htmx 4](https://four.htmx.org/docs/whats-new-in-htmx-4).
- Header reference pages: [HX-Trigger](https://four.htmx.org/reference/headers/HX-Trigger), [HX-Redirect](https://four.htmx.org/reference/headers/HX-Redirect), [HX-Location](https://four.htmx.org/reference/headers/HX-Location).
- **Source reading** of `htmx.org@4.0.1/dist/htmx.js` (unpkg). Statements marked *(source)* were checked in that file. They were not exercised in a browser.

## 1. Request headers

| Header | htmx 2 | htmx 4 | Notes for the server |
|---|---|---|---|
| `HX-Request` | `true` | `true` | Present on every htmx request, including history restores. |
| `HX-Request-Type` | — | `partial` / `full` | **New.** *(source)* `full` when the target is `<body>` or `hx-select` is used, otherwise `partial`. Boosted requests, history restores and `HX-Location` follow-ups target `<body>` by default, so they are `full`. |
| `HX-Target` | target element **id** | target as **`tag#id`** (e.g. `div#results`) | Format changed. *(source)* the id part is `encodeURI`d; the header is set whenever a target resolves. |
| `HX-Trigger` (request) | triggering element **id** | — | Replaced by `HX-Source`. |
| `HX-Source` | — | source element as `tag#id` | **New.** *(source)* the format is `tagName.toLowerCase()` + `#` + `encodeURI(id)`, or only the tag name if there is no id. |
| `HX-Trigger-Name` | `name` of trigger | — | **Removed.** |
| `HX-Current-URL` | browser URL | browser URL | Unchanged. |
| `HX-Boosted` | `true` | `true` | Unchanged. |
| `HX-History-Restore-Request` | `true` | `true` | In v4, sent when htmx re-fetches a page on back/forward. |
| `HX-Prompt` | prompt answer | only via `hx-prompt` extension | Moved to an extension. |
| `Accept` | browser default | `text/html` | *(source)* core requests send `text/html`; `hx-sse` adds `text/event-stream`. |

**Design consequences:**

- **Full vs fragment:** in v4, decide on `HX-Request-Type`, not on `HX-Request`.
  - With htmx 2 the server had to guess. Alex Edwards recommends `HX-Redirect` over `HX-Location` because the target handler "has no way of telling" whether to send a full page.
  - In htmx 4 *(source)*, `HX-Location` follow-ups, boosted navigation, history restores and `hx-select` requests all say `full`.
- **Version-normalised model:** for htmx 2 clients (no `HX-Request-Type`), approximate "partial" as `HX-Request == "true"` and not boosted and not a history restore.
- **Element identifiers:** parse `tag#id` with `strings.Cut(v, "#")`, then `url.PathUnescape` the id. (`encodeURI` never emits `#`, and tag names can't contain it.) For v2, the id is the whole value of `HX-Target` or `HX-Trigger`.

## 2. Response headers

| Header | htmx 2 | htmx 4 | Notes |
|---|---|---|---|
| `HX-Location` | ✓ | ✓ | Client-side navigation without a full reload (an ajax GET + push). Accepts a path or a JSON object. |
| `HX-Redirect` | ✓ | ✓ | Full-page navigation. |
| `HX-Refresh` | ✓ | ✓ | `"true"` → full reload. |
| `HX-Push-Url` / `HX-Replace-Url` | ✓ | ✓ | |
| `HX-Reswap` / `HX-Retarget` / `HX-Reselect` | ✓ | ✓ | |
| `HX-Trigger` | ✓ | ✓ **fires after the swap** | Accepts an event name, a comma list, or JSON. **New in v4:** a `"target"` key in the event detail dispatches on another element, e.g. `{"cartUpdated":{"target":"#cart","count":3}}`. *(source)* handled in `finally`, after the swap. |
| `HX-Trigger-After-Swap` | ✓ | **removed** | Use `HX-Trigger`, which now fires after the swap anyway. |
| `HX-Trigger-After-Settle` | ✓ | **removed** | Same. |

**Processing rules that matter for a Go helper:**

1. **"Response headers are not processed on 3xx response codes. Return a 2xx status when using this header."** ([HX-Redirect](https://four.htmx.org/reference/headers/HX-Redirect) and [HX-Location](https://four.htmx.org/reference/headers/HX-Location) docs.) An htmx-aware redirect must use `200`/`204` + header, never `http.Redirect`'s 3xx.
2. *(source)* precedence: `HX-Refresh` → `HX-Redirect` → `HX-Location` each **short-circuit**, so no swap happens. Then `HX-Retarget`/`HX-Reswap`/`HX-Reselect` modify the swap, and `HX-Trigger` runs last.
3. *(source)* all `hx-*` response headers are read case-insensitively. Go's `http.Header.Set` canonicalises to `Hx-Trigger`, which is fine.
4. Headers must be set **before** `WriteHeader`. That's another reason to buffer the body (see [stdlib-html-template.md §4](stdlib-html-template.md#4-failure-modes-at-runtime)).

## 3. Behaviour changes that affect server code

| Change | htmx 2 | htmx 4 | Server impact |
|---|---|---|---|
| Attribute inheritance | implicit | **explicit** (`hx-target:inherited`, `hx-headers:inherited`, …) | Layout templates that put `hx-headers`/`hx-target` on `<body>` must add `:inherited`. Restore the old behaviour with `htmx.config.implicitInheritance = true`. |
| Error responses | 4xx/5xx **not swapped** | **swapped** (`noSwap` defaults to `[204, 304]`) | Every error response becomes visible UI by default. The server must return a **fragment that makes sense in the target**, or retarget. Per element: `hx-status:422="target:#errors"`, `hx-status:5xx="swap:none push:false"`. Global: `noSwap: [204, 304, "4xx", "5xx"]` restores v2. |
| History | DOM snapshots in `sessionStorage` | **re-fetches the URL** (`HX-History-Restore-Request: true`, `full`) | Any URL that was pushed must return a full page. That was always the rule; now it is exercised on every back/forward. The opt-in `hx-history-cache` extension restores snapshots. |
| OOB swaps | before the main swap | **after the main swap** | OOB fragments can target elements the main swap just created. |
| Multi-target | OOB only | **`<hx-partial hx-target="#id" hx-swap="…">`** (or `<template hx type="partial">`) | Cleaner multi-fragment responses. A response with **only** `<hx-partial>` elements skips the main swap. `id="x"` is shorthand for `hx-target="#x"`. |
| Polling stop | status `286` stops polling | *(source)* **`286` not recognised** | Use load-polling (the element replaces itself) or a filter. The `hx-ptag` extension skips unchanged polls. |
| SSE / WebSockets / multipart | extensions (htmx-ext-sse, -ws) | extensions (`hx-sse`, `hx-ws`, `hx-multipart`) | An SSE endpoint is still plain `text/event-stream`. |
| `hx-disable` | skip htmx processing | renamed to **`hx-ignore`**; `hx-disable` now means what `hx-disabled-elt` meant | Template migration only. |
| `hx-trigger` `queue:` modifier | supported | removed (use `hx-sync="this:queue all"`) | Template migration only. |
| `hx-swap` / `HX-Reswap` modifiers | kebab-case, e.g. `focus-scroll:true`, `show:#list:top` | camelCase keys (`focusScroll`, `ignoreTitle`, `swapEmpty`). `show`/`scroll` take only `top`/`bottom`, with the selector given separately. | **Server-relevant:** an `HX-Reswap` builder must render version-specific syntax. go-htmx renders `show:top showTarget:"#list" focusScroll:true` for v4. |
| CSRF example | `hx-headers` on body | `hx-headers:inherited='js:{"X-CSRF-Token": …}'` on body | Inheritance is explicit. See CSRF in [stdlib-html-template.md §5](stdlib-html-template.md#5-security-pitfalls-with-htmx-attributes). |
| Caching | `Vary: HX-Request` advised | same advice | With `HX-Request-Type`-based branching, also vary on `HX-Request-Type`. |

htmx 4 also ships an upgrade checker that scans templates. By default it scans `.html`, `.php`, `.js`, `.ts`, `.jinja*`, `.erb` and `.hbs`, so Go template suffixes must be passed explicitly:

```sh
npx htmx.org@4.0.1 upgrade-check --ext .tmpl ./path/to/templates
```

## 4. Default client configuration for an htmx-4 app

Alex Edwards' v2 config ([stdlib-html-template.md §2](stdlib-html-template.md#2-the-canonical-stdlib--htmx-pattern-alex-edwards-2026)) mostly turned v2 into v4-like behaviour. Mapped to v4:

| v2 setting (Alex Edwards) | In v4 |
|---|---|
| `disableInheritance: true` | Default (explicit inheritance) |
| `historyCacheSize: 0` | Default (no snapshot cache) |
| `historyRestoreAsHxRequest: false` | Not needed; restores are `HX-Request-Type: full` *(source)* |
| `responseHandling` with 422 swap and `[45]..` → body | 4xx/5xx are swapped by default; per-code targets via `hx-status:*` |
| `includeIndicatorStyles: false` | **Renamed** to `includeIndicatorCSS` *(source)*. The old key does nothing in v4. |
| `timeout: 5000` | **Renamed** to `defaultTimeout` *(source)*. The v4 default is `60000` ms (in v2 the default was no timeout). |

Other v4 defaults worth knowing *(source)*: `mode: 'same-origin'` (fetch mode, so cross-origin requests fail unless reconfigured), `defaultSwap: "innerHTML"`, `noSwap: [204, 304]`, `implicitInheritance: false`, and `prefix: "data-hx-"` (`data-hx-*` works next to `hx-*`).

**Inference:** v4 most likely ignores unknown config keys silently, so a v2 config pasted into a v4 app is a quiet migration trap.

**Implication (inference):** an adapter-provided `<meta name="htmx-config">` helper for v4 can be nearly empty. Its main job becomes documenting the 4xx/5xx swap behaviour, and making the HTML error handler emit sensible fragments (see [adapter-design-implications.md](adapter-design-implications.md)).

## 5. Server checklist (v2 and v4)

- [ ] Choose full page vs fragment from `HX-Request-Type` (v4), falling back to `HX-Request` && !boosted && !history-restore (v2).
- [ ] Always set `Vary: HX-Request, HX-Request-Type` on HTML responses whose body depends on them.
- [ ] Redirect htmx requests with a 2xx + `HX-Redirect`/`HX-Location`; use `http.Redirect` (3xx) only for non-htmx requests.
- [ ] Set all `HX-*` response headers before `WriteHeader`; buffer the body.
- [ ] Merge multiple `HX-Trigger` events into one JSON object; don't send `HX-Trigger-After-*` to v4 clients.
- [ ] Error responses (4xx/5xx) must be swappable fragments for htmx requests (v4), or full pages for full requests.
- [ ] Every pushed URL must render a full page on a direct request.
- [ ] Multi-region updates: OOB (`hx-swap-oob`) for v2 and v4, `<hx-partial>` for v4 only.
- [ ] Don't rely on status 286 to stop polling.
- [ ] Parse `HX-Target`/`HX-Source` as `tag#id` (v4) or bare id (v2).
