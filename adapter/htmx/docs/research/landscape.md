# Landscape: Go front-end and server-side HTML packages

All numbers were collected on **2026-11-16**:

- **Stars** and **last push** come from the GitHub REST API (`api.github.com/repos/...`).
- **Importers** is the "Imported by" count on `pkg.go.dev` for the given package path.

Read the numbers with these caveats:

- Stars measure attention, not usage. Importer counts are a better usage proxy, but they only see public modules indexed by pkg.go.dev, and they count *packages*, not projects.
- Where a library changed its module path (`v2`, `v3`, vanity domains), the counts are split between paths. The relevant paths are listed.
- "n/a" means the number was not collected.

## 1. Server-side HTML rendering

This is the core of the question: how Go developers produce HTML.

| Package | Approach | Stars | Importers | Last push | Notes |
|---|---|---:|---:|---|---|
| `html/template` (stdlib) | text templates with contextual auto-escaping | n/a | **118,327** | n/a | `text/template`: 123,258. Counts all uses, not only web pages. |
| [a-h/templ](https://github.com/a-h/templ) | `.templ` DSL compiled to Go (codegen) | **10,613** | **7,591** (`templ/runtime`: 2,941) | 2026-11-13 | Clear #1 third-party option. LSP, fragments, html/template interop. |
| [valyala/quicktemplate](https://github.com/valyala/quicktemplate) | template DSL compiled to Go | 3,327 | 671 | 2026-10-20 | Performance-oriented, older generation. |
| [flosch/pongo2](https://github.com/flosch/pongo2) | Django/Jinja-like syntax | 3,085 | 185 (v6) + 101 (v4) | 2026-05-02 | Appeals to Python/Django migrants. |
| [unrolled/render](https://github.com/unrolled/render) | render helper on top of `html/template` (+JSON/XML) | 2,002 | 1,623 | 2026-09-14 | Popular "thin layer over stdlib". |
| [maragudk/gomponents](https://github.com/maragudk/gomponents) | HTML as plain Go function calls | 1,925 | 468 (`maragu.dev/gomponents`) | active | No build step, zero deps. A distant #2 among "type-safe" options. |
| [yann-shi/hero](https://github.com/yann-shi/hero) (was shiyanhui/hero) | codegen templates | 1,567 | n/a | 2020-01-09 | Dormant. |
| [CloudyKit/jet](https://github.com/CloudyKit/jet) | own template language | 1,411 | 391 (v6) | 2026-09-24 | |
| [gobuffalo/plush](https://github.com/gobuffalo/plush) | ERB-like (Buffalo) | 1,027 | 98 (v4) + 9 (v5) | 2026-10-31 | Tied to the Buffalo framework (2% usage, JetBrains 2026). |
| [maddalax/htmgo](https://github.com/maddalax/htmgo) | Go builder + htmx framework | 941 | 14 (`framework/h`) | 2025-09-27 | HN launch: 166 points. Low real adoption. |
| [eknkc/amber](https://github.com/eknkc/amber) | Pug/Haml-like | 918 | 276 | 2020-10-13 | Dormant. |
| [foolin/goview](https://github.com/foolin/goview) | layout helper over `html/template` | 465 | 97 | 2025-08-10 | |
| [chasefleming/elem-go](https://github.com/chasefleming/elem-go) | HTML as Go calls, with htmx helpers | 366 | 14 | 2026-10-30 | |
| [gofiber/template](https://github.com/gofiber/template) | Fiber view-engine adapters | 313 | 454 (`html/v2`) | 2026-11-12 | Most Fiber users pick the `html/template` engine. |
| [doors-dev/gox](https://github.com/doors-dev/gox) | HTML as Go expressions, with editor support | 66 | n/a | 2026-11-14 | New (created 2025-12). |
| [gsxhq/gsx](https://github.com/gsxhq/gsx) | JSX-like `.gsx` compiled to Go, streaming HTML | n/a | n/a | n/a | Alpha (`v0.1.0`+). The author [says](https://news.ycombinator.com/item?id=48918100) they built it after templ's ergonomics fell short in production. |
| [donseba/go-partial](https://github.com/donseba/go-partial) | request-aware partial rendering over `html/template` | 24 | n/a | 2026-11-01 | Very broad feature set. See [go-htmx-helper-libraries.md](go-htmx-helper-libraries.md). |
| [alesr/templator](https://github.com/alesr/templator) | type-safe wrapper over `html/template` | 13 | n/a | 2026-11-05 | |
| [jba/templatecheck](https://github.com/jba/templatecheck) | static type-checking of `html/template` | 69 | n/a | 2024-10-24 | By a Go team member. A niche but telling need. |

### Template function libraries for stdlib templates

| Package | Stars | Importers | Last push |
|---|---:|---:|---|
| [Masterminds/sprig](https://github.com/Masterminds/sprig) | 4,752 | 1,791 (v3) | 2025-07-18 |
| [go-sprout/sprout](https://github.com/go-sprout/sprout) (sprig successor) | 233 | n/a | 2026-11-16 |

Sprig's numbers show how much demand there is for "the missing funcs" (`dict`, `default`, string helpers) on top of stdlib templates. A meaningful share of that comes from Helm and other `text/template` users, though, not only from web UIs.

## 2. htmx and hypermedia helper libraries for Go

| Package | What it does | Stars | Importers | Last push | htmx 4 support |
|---|---|---:|---:|---|---|
| [angelofallars/htmx-go](https://github.com/angelofallars/htmx-go) | request checks, fluent response-header builder, templ helper | 807 | 41 | **2024-11** (stale) | No (2.x headers only) |
| [jfyne/live](https://github.com/jfyne/live) | LiveView-style components over websockets | 749 | 46 | 2026-04-13 | n/a (own JS) |
| [yznts/kyoto](https://github.com/yznts/kyoto) | component framework | 656 | n/a | 2026-03-24 | n/a |
| [donseba/go-htmx](https://github.com/donseba/go-htmx) | request checks, response headers, components over `html/template`, SSE | 603 | 19 | 2026-11-05 | **Yes, opt-in** (`SetVersion`) |
| [starfederation/datastar-go](https://github.com/starfederation/datastar-go) | official Datastar SDK (SSE patches, signals) | 212 | 113 | 2026-06-02 | n/a (Datastar) |
| [livefir/fir](https://github.com/livefir/fir) | Go + Alpine reactive apps | 180 | n/a | archived | n/a |
| [maragudk/gomponents-htmx](https://github.com/maragudk/gomponents-htmx) | htmx attributes and helpers for gomponents | 123 | 42 (`maragu.dev/gomponents-htmx`) | 2024-10-11 | Unknown (not verified) |
| [will-wow/typed-htmx-go](https://github.com/will-wow/typed-htmx-go) | typed attribute builders | 11 | n/a | 2024-05-27 | No |

The niche is small and fragmented. The two best-known header helpers have 600–800 stars but only **19–41 importers**. Most people hand-roll a handful of helpers instead (see [stdlib-html-template.md](stdlib-html-template.md)).

## 3. Starter kits and "stacks"

| Repo | Stack | Stars | Last push |
|---|---|---:|---|
| [mikestefanello/pagoda](https://github.com/mikestefanello/pagoda) | Go + htmx + Alpine + Tailwind. Started on `html/template`; current versions use gomponents. | 2,964 | 2026-10-30 |
| [axadrn/shadcn-templ](https://github.com/axadrn/shadcn-templ) (formerly templUI; `templui/templui` redirects here) | shadcn-style component kit for templ | 1,797 | 2026-11-16 |
| [gowebly/gowebly](https://github.com/gowebly/gowebly) | CLI generator: Go + htmx + templ | 1,234 | 2026-04-18 |
| [TomDoesTech/GOTTH](https://github.com/TomDoesTech/GOTTH) | Go + templ + Tailwind + htmx | 685 | 2026-05-28 |
| [leomorpho/goship](https://github.com/leomorpho/goship) | SaaS starter (htmx, templ) | 458 | n/a |
| [emarifer/go-echo-templ-htmx](https://github.com/emarifer/go-echo-templ-htmx) | Echo + templ + htmx demo | 297 | 2024-06-19 |
| [romshark/templier](https://github.com/romshark/templier) | templ dev server with live reload | 121 | 2026-11-14 |
| [romshark/datapages](https://github.com/romshark/datapages) | Datastar framework and codegen | 121 | 2026-11-15 |
| [air-verse/air](https://github.com/air-verse/air) | generic Go live-reload (widely used for template dev loops) | 24,125 | 2026-08-27 |
| [olivere/vite](https://github.com/olivere/vite) | Vite backend integration for Go | 216 | 2025-06-16 |

pagoda's move from `html/template` to gomponents is the best-documented case of a Go web starter leaving the stdlib. The README's ["Why Gomponents?"](https://github.com/mikestefanello/pagoda#why-gomponents) section explains it:

- The old template tooling (layout/page compilation, component auto-include, htmx partial rendering, funcmaps) is archived on the [`templates` branch](https://github.com/mikestefanello/pagoda/tree/templates).
- The reason given: "Templates provide no type-safety, child templates are difficult to call when you have multiple arguments, templates are not flexible enough to easily provide reusable components and elements, the _funcmap_ and form submission code often had to return HTML or CSS classes, and more."
- templ was considered and rejected because it "requires you to know and understand their templating language, to install a CLI and an IDE plugin (which does not work with all IDEs; especially GoLand), and separately compile template code."

The author was "extremely hesitant to adopt a rendering option outside the standard library". The pain points they list are the ones an `html/template`-first adapter has to solve; see [stdlib-html-template.md](stdlib-html-template.md).

GitHub topic counts:

| Search | Repositories |
|---|---:|
| `topic:htmx language:go` | 806 |
| `topic:templ` | 314 |
| Full-text search `htmx language:go` | ~3,062 |

Among the top-15 `topic:htmx` Go repos, templ appears far more often than any other rendering approach. Several of them, such as glasskube, openrun and statusnook, are real products, not demos.

## 4. Client-side hypermedia and JS companions

| Library | Stars | Last push | Notes |
|---|---:|---|---|
| [bigskysoftware/htmx](https://github.com/bigskysoftware/htmx) | **49,645** | 2026-11-13 | **v4.0.0 released 2026-08-28**, v4.0.1 on 2026-11-01. 2.x is still maintained (v2.0.11 on 2026-11-01). 0BSD license. `htmx.min.js` 4.0.1 is 37,448 B, about 13.4 KB gzipped. |
| [alpinejs/alpine](https://github.com/alpinejs/alpine) | 32,023 | 2026-11-04 | The usual "client-side sprinkles" companion to htmx. htmx 4 ships an Alpine compat extension. |
| [hotwired/turbo](https://github.com/hotwired/turbo) | 7,400 | 2026-11-15 | No notable Go-specific integration found. |
| [starfederation/datastar](https://github.com/starfederation/datastar) | 5,249 | 2026-11-09 | SSE-first hypermedia with signals. Created by a Go developer, with a Go SDK. Big HN traction in 2025. |
| [unpoly/unpoly](https://github.com/unpoly/unpoly) | 2,798 | 2026-11-16 | |

## 5. Go in the browser, and desktop

| Project | Stars | Importers | Last push | Notes |
|---|---:|---:|---|---|
| [wailsapp/wails](https://github.com/wailsapp/wails) | 37,127 | n/a | 2026-11-16 | Desktop apps with a web front-end (usually a JS framework). |
| [maxence-charriere/go-app](https://github.com/maxence-charriere/go-app) | 8,984 | 110 (v10) + 320 (v9) | 2026-11-03 | WASM PWA framework. |
| [vugu/vugu](https://github.com/vugu/vugu) | 5,011 | 95 | 2026-09-23 | WASM UI. |
| [hexops/vecty](https://github.com/hexops/vecty) | 2,900 | n/a | archived | |

WASM UI in Go gets attention (stars) but little usage (importers). It isn't the space an htmx adapter competes in.

## 6. Routers, for context

| Router | Stars | Importers | JetBrains 2026 "regularly use" |
|---|---:|---:|---:|
| `net/http` (stdlib) | n/a | n/a | **46%** (32% in 2025) |
| [gin-gonic/gin](https://github.com/gin-gonic/gin) | 89,525 | 191,418 | 48% |
| [gofiber/fiber](https://github.com/gofiber/fiber) | 40,282 | n/a | 13% |
| [labstack/echo](https://github.com/labstack/echo) | 32,805 | 32,160 (v4) | 13% |
| [go-chi/chi](https://github.com/go-chi/chi) | 23,109 | 17,309 (v5) | 15% |
| gorilla/mux | n/a | n/a | 15% (archived in 2022) |

An adapter that is plain `http.Handler`/`net/http` fits every row of this table.

## 7. Supporting concerns that htmx apps always need

| Concern | Options (stars / importers) |
|---|---|
| CSRF | stdlib `http.CrossOriginProtection` (**Go 1.25+**, `Sec-Fetch-Site`/`Origin` based); [justinas/nosurf](https://github.com/justinas/nosurf) 1,757 / 2,530; [gorilla/csrf](https://github.com/gorilla/csrf) 1,213 / 1,103 |
| Form decoding | [gorilla/schema](https://github.com/gorilla/schema) 1,517 / 2,946; [go-playground/form](https://github.com/go-playground/form) 934 / 637; frameless already ships `pkg/httpkit/formurlencoded` |
| Static assets | stdlib `embed` + `http.FileServerFS` (Go 1.22+) |
| Live reload | air (24k stars), templier, `templ generate --watch` |

## Takeaways

1. **Stdlib is the baseline by an order of magnitude.** `html/template` has ~15× the importers of templ. That number is inflated by non-web uses, but no third-party engine comes close.
2. **Among third-party options it's templ, then everything else.** templ has ~10.6k stars, ~7.6k importers, and its own component and tooling ecosystem. gomponents is the "no codegen" alternative. The classic engines (quicktemplate, pongo2, jet, plush, amber, hero) are stable or dormant, with little momentum.
3. **htmx is the default interactivity layer for Go server-rendered apps.** Alpine is its companion and Datastar the rising challenger. Turbo and Unpoly barely register in the Go community.
4. **The htmx helper library niche is under-served and fragmented.** The most-starred helper is stale and pre-htmx-4. The most active one, donseba/go-htmx, has 19 importers.
