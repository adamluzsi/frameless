# Research: which front-end packages Go developers prefer, and what that means for an `htmx` adapter

**Question:** What front-end and web-UI packages do Go developers like most, especially stdlib `html/template` usage? How should that shape a frameless `adapter/htmx` package for building dynamic web pages?

**Date:** 2026-11-16 · **Go:** 1.27.3 · **htmx:** 4.0.1 (2.0.11 still maintained)

## TL;DR

1. **Stdlib `html/template` is the default by a wide margin**, and many Go developers want to stay on it. It has ~118k importers vs ~7.6k for templ, the #1 third-party engine. The top reasons given are "built-in" and "secure by default". → [landscape.md](landscape.md), [community-sentiment.md](community-sentiment.md)
2. **The complaints about stdlib are ergonomic, not about capability:**
   - Organising layouts, pages and partials (the `Clone()` debate).
   - Passing several arguments to components.
   - Errors that only surface at runtime.
   - Missing helper funcs.
   - Weak editor support.
   - Fragment and OOB plumbing for htmx.

   pagoda left `html/template` for gomponents over exactly these issues, and documented why. → [stdlib-html-template.md](stdlib-html-template.md)
3. **templ is the type-safe alternative.** It has momentum (10.6k stars, starter kits, a UI kit, conference workshops). Its CLI, codegen and editor plugin push a visible minority back to stdlib or to gomponents (no codegen).
4. **htmx is the default interactivity layer** for server-rendered Go. Alpine is its companion and Datastar the energetic challenger. TypeScript SPAs remain the most common pairing overall: JetBrains 2026 reports 43% of Go projects with TypeScript.
5. **htmx 4 (released 2026-08-28) changed the server contract.** Changes include:
   - New `HX-Request-Type` (`full`/`partial`) and `HX-Source` headers.
   - `tag#id` element identifiers.
   - 4xx/5xx swapped by default.
   - History restore re-fetches full pages.
   - `<hx-partial>` multi-target responses.
   - `HX-Trigger-After-*` and status 286 removed.
   - Swap modifiers renamed.
   - Config keys renamed.

   Most Go tutorials and helpers still target htmx 2. → [htmx-4-server-contract.md](htmx-4-server-contract.md)
6. **The Go htmx helper niche is small, fragmented and mostly pre-v4.** The most-starred helper (htmx-go, 807 stars) has been stale since 2024. The only maintained one with v4 support (go-htmx) has 19 importers and v4 is opt-in. None of them integrate error mapping, data access or testing. → [go-htmx-helper-libraries.md](go-htmx-helper-libraries.md)
7. **Security finding (verified on Go 1.27.3):** `html/template` treats `hx-on:*` (and Alpine `x-on:*`/`@*`/`x-data`) as plain attributes. It only HTML-escapes them, and the browser decodes that back before htmx or Alpine runs the value as JS, so **interpolating untrusted data there is an XSS vector**. `hx-get` and friends are not URL context either: `javascript:` and `../` pass through. A `json` func and a pathkit-based `path` func fix both. → [stdlib-html-template.md §5](stdlib-html-template.md#5-security-pitfalls-with-htmx-attributes)

## Recommendation (summary)

Build `adapter/htmx` as a **stdlib-first, htmx-4-first, frameless-native** package. Full proposals, sketches and open questions are in [adapter-design-implications.md](adapter-design-implications.md).

1. A **version-normalised `Request`** (v2 and v4 detected per request) and a **value-typed `Response`** header model (trigger merging, version-specific syntax).
2. **Middleware** that sets `Vary: HX-Request, HX-Request-Type` and puts the request model in ctx. **Redirect helpers** that never send `HX-*` headers on 3xx.
3. A **`Component` interface** (`Render(ctx, io.Writer) error`). templ satisfies it as-is, and gomponents needs a one-line adapter.
4. A **stdlib template set over `fs.FS`**:
   - Layout/partials/pages conventions.
   - Per-page clones built at startup.
   - Buffering, and `missingkey=error`.
   - Safe funcs (`json`, `path`, `asset`, `dict`).
   - Full-vs-fragment selection and `<hx-partial>`/OOB multi-target responses.
   - **Startup checks** (`Check`).
5. An **HTML `ErrorHandler`** that reuses `rfc7807.Handler.ToDTO` + `httpkit.ErrorMapping`, so HTML and JSON endpoints agree on status codes. It renders swappable fragments for partial requests, which matters now that v4 swaps 4xx/5xx.
6. **`App.Mount`** serving vendored htmx (0BSD, ~13.4 KB gz) under hashed, immutable URLs, plus a config `<meta>` helper.
7. **`htmxtest`**: request builders with htmx v2/v4 headers, response parsing, and template checks.
8. **Later:** an HTML resource handler built from `crud`-shaped funcs, and an SSE bridge to pub/sub.
9. **Non-goals:** a component DSL, a templ/gomponents/router dependency, a Node build, go-partial-style scope.

## Files

| File | Contents |
|---|---|
| [landscape.md](landscape.md) | Package-by-package numbers: rendering engines, htmx helpers, starter kits, client libraries, WASM, routers, supporting concerns |
| [community-sentiment.md](community-sentiment.md) | Go Developer Survey and JetBrains 2026 data; Hacker News threads, with verbatim, linked quotes grouped by theme |
| [stdlib-html-template.md](stdlib-html-template.md) | How developers use `html/template` with htmx, the canonical pattern, template-set organisation, runtime failure modes, **verified security pitfalls**, fragments, interop |
| [htmx-4-server-contract.md](htmx-4-server-contract.md) | htmx 2 → 4 request/response header and behaviour changes that affect Go servers, plus a server checklist |
| [go-htmx-helper-libraries.md](go-htmx-helper-libraries.md) | API comparison of go-htmx, htmx-go, pagoda `pkg/htmx`, gomponents-htmx, go-partial, webtools, templ fragments, datastar-go |
| [adapter-design-implications.md](adapter-design-implications.md) | Proposals for the frameless adapter: API sketches, frameless integration points, phasing, non-goals, open questions |

## Methodology

- **Adoption data:** the GitHub REST API (stars, last push, archived status) and the pkg.go.dev "Imported by" counts, collected 2026-11-16. Where the API's anonymous rate limit was exhausted, values are "n/a" instead of estimated.
- **Surveys:** the official Go Developer Survey reports (2023 Q1 – 2025) and the JetBrains "State of Go 2026" post.
- **Community discussion:** Hacker News via the Algolia HN API. Thread points and comment counts were re-fetched, and every quote was matched to its comment id. Mention counts were computed over comment text.
- **htmx 4 behaviour:**
  - The official docs (`four.htmx.org`, `llms-full.txt`, "What's new in htmx 4", header reference pages).
  - **Reading `htmx.org@4.0.1/dist/htmx.js`**. Claims marked *(source)* come from that reading and were not exercised in a browser.
- **`html/template` behaviour:** small Go programs run on Go 1.27.3; outputs are pasted verbatim. pathkit behaviour came from a throwaway test in `pkg/pathkit`, which was deleted afterwards.
- **Library APIs:** READMEs and source from `raw.githubusercontent.com` on default branches.
- **frameless facts** (`httpkit`, `rfc7807`, `formurlencoded`, `codec`, `crud`, `RESTHandler`): read from this repository.

## Caveats

- **Source bias:** Reddit (r/golang, r/htmx), Discord, and general web search weren't reachable from the research environment. Community sentiment is therefore Hacker-News-heavy, and HN skews toward experienced, opinionated developers.
- **Proxies:** stars measure attention, not use. Importer counts only see public modules and count packages, not projects. `html/template`'s count includes non-web uses (emails, generated HTML reports).
- **Surveys:** both are self-selected samples. JetBrains' sample (1,519 for the router question) and the Go survey's samples differ, so compare trends within a survey, not across them.
- **Fast-moving area:** htmx 4 is three months old. Helper libraries and tutorials are mid-migration, so expect the v4 adoption picture to change quickly.
- **Inference vs fact:** statements labelled *Inference* or *proposal* are judgement calls. Everything else is cited or reproduced.
