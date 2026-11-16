# Community sentiment: what Go developers say about building HTML front-ends

This file covers what Go developers *say* and *report*. For what they *use* (stars, importers, last push), see [landscape.md](landscape.md).

Sources were collected on 2026-11-16. Quotes are verbatim and link to the exact comment. Hacker News data comes from the Algolia HN API.

Caveats:

- Reddit (r/golang, r/htmx) and general web search were not reachable from the research environment, so this file is biased toward Hacker News and official surveys.
- Survey numbers are self-reported, and each survey had a different sample size.

## 1. Survey data

### Go Developer Survey (Go team)

Share of respondents who say they build **"Websites / web services (returning HTML)"**:

| Survey | Share | n |
|---|---:|---:|
| [2023 Q1](https://go.dev/blog/survey2023-q1-results) | 37% | 5,838 |
| 2023 H2 | 42% | 3,655 |
| 2024 H1 | 45% | 5,768 |
| 2024 H2 | 45% | 3,769 |
| 2025 | 46% | 2,502 |

From the 2023 Q1 report:

- For **novice** Go developers, websites returning HTML was the #1 area where they wanted to use Go more.
- Of the 29 open-text answers about what blocked them, 12 said other languages have better frameworks for it. One example: *"It's usually easier to accomplish this in other languages such as PHP or Ruby…"*

**Reading:** about 4 in 10 Go developers ship HTML, and the share went up steadily from 2023 to 2025. Where developers felt something was missing, they cited frameworks and ergonomics, not language capability.

### JetBrains "State of Go 2026"

Source: [JetBrains, State of Go 2026](https://blog.jetbrains.com/go/2026/11/10/state-of-golang-2026/).

- **Routers** (base 1,519):

  | Router | Share |
  |---|---:|
  | Gin | 48% |
  | `net/http` | **46%** (up from 32% the previous year) |
  | chi | 15% |
  | gorilla/mux | 15% |
  | Echo | 13% |
  | Fiber | 13% |
  | Beego | 5% |
  | Buffalo | 2% |
  | None | 12% |

- **Full-stack web development** rose from 30% to **40%**. Backend web is 80%.
- **TypeScript as an in-project companion** went from 24% (2024) to 31% to **43% (2026)**. Go-only projects fell from 35% to 27%. The report says *"Go backend + TS frontend is turning into the default stack."*
- The report speculates that AI coding agents favour the standard library, and links that to the `net/http` jump.
- GopherCon Africa ran a hands-on workshop on **sqlc + templ**.

**Reading:** two trends pull in different directions:

- More Go developers do full-stack work, and stdlib `net/http` is gaining fast.
- The most common pairing is still a TypeScript SPA front-end.

A server-rendered htmx stack is the main alternative to that. It competes on simplicity, not on popularity.

## 2. Hacker News threads

| Thread | Date | Points | Comments |
|---|---|---:|---:|
| [How I use HTMX with Go](https://news.ycombinator.com/item?id=48912175) (Alex Edwards) | 2026-07-14 | 315 | 114 |
| [Dependency management fatigue, or why I ditched React for Go+HTMX+Templ](https://news.ycombinator.com/item?id=42305348) | 2024-12-03 | 273 | 300 |
| [Templ: A language for writing HTML user interfaces in Go](https://news.ycombinator.com/item?id=38597599) | 2023-12-11 | 228 | 98 |
| [Show HN: htmgo – build simple and scalable systems with golang + htmx](https://news.ycombinator.com/item?id=41683144) | 2024-09-28 | 166 | 81 |
| [Show HN: Build progressively enhanced reactive HTML apps using Go and Alpine.js](https://news.ycombinator.com/item?id=35881963) (fir) | 2023-05-10 | 127 | 43 |
| [Show HN: Gomponents, HTML components in pure Go](https://news.ycombinator.com/item?id=41847587) | 2024-10-15 | 113 | 41 |
| [Show HN: Use Go's HTML/template to write React-like code](https://news.ycombinator.com/item?id=40490608) | 2024-05-27 | 106 | 95 |
| [Show HN: templUI – The UI Kit for templ](https://news.ycombinator.com/item?id=44132756) | 2025-05-30 | 62 | 34 |
| [I switched from Htmx to Datastar](https://news.ycombinator.com/item?id=45536000) | 2025-10-10 | 315 | 380 |
| [Datastar: Lightweight hypermedia framework…](https://news.ycombinator.com/item?id=45536618) | 2025-10-10 | 306 | 287 |

In the 2026 Alex Edwards thread, the number of comments mentioning each option was:

| Option | Comments |
|---|---:|
| Datastar | 9 |
| templ | 7 |
| `html/template` (explicitly named) | 4 |

The article itself is a pure `html/template` tutorial.

## 3. Recurring themes, with quotes

### 3.1 "Stdlib templates are good enough", a pragmatic majority voice

- **shimman** ([comment](https://news.ycombinator.com/item?id=48914317)): *"I tried to use templ but it felt more frustrating than ergonomic. Like at that point I'd rather just use react (and I hate react). Just sticking with basic std templates + HTMX sprinkling is good enough for my needs."*
- **flashgordon** ([comment](https://news.ycombinator.com/item?id=40492120)): went from server-side templating to Next.js/React and *"then back to htmx/go templates"*. In the gomponents thread ([1](https://news.ycombinator.com/item?id=41875515), [2](https://news.ycombinator.com/item?id=41940181)) they swing between templates and typed approaches. They were wary of templ's build step and of tying templating to one syntax.
- **jackielii** ([comment](https://news.ycombinator.com/item?id=48918100)): *"On html/template, I like the security by default, and obviously it's built-in."*

### 3.2 …but `html/template`'s ergonomics are the #1 complaint

- **localhostinger** ([comment](https://news.ycombinator.com/item?id=48917022)): *"As for Go's html/template: I think it has one of the weirdest / most unnatural interfaces."*
- **Zababa** ([comment](https://news.ycombinator.com/item?id=48917769)): *"I've tried to like Go with HTMX, but the big issue was always Go templates. I feel like if there was something like JSX/TSX but for Go, it would be a way better dev experience…"*
- **jerf** ([comment](https://news.ycombinator.com/item?id=38603129)): *"Standard Go templating can do almost anything you want it to do, but figuring out how to do it from the documentation alone can be a real challenge… I like the attempt at making templating safe, but it could really use a v2."*
- **markusw**, the gomponents author ([comment](https://news.ycombinator.com/item?id=41876471)): *"I started with html/template, but got fed up with it because I thought it was so hard to pass data around to different components."*
- **pagoda**'s README makes the same argument at length (see [landscape.md §3](landscape.md#3-starter-kits-and-stacks)): no type safety, child templates are hard to call with multiple arguments, and components aren't reusable.

### 3.3 Errors only show up at runtime

- **thraxil** ([1](https://news.ycombinator.com/item?id=38601195), [2](https://news.ycombinator.com/item?id=38601482)) showed a template typo (`Titl` instead of `Title`): *"Program compiles. Parse() succeeds. It just fails at runtime."* With templ, *"it gets caught at compile time"*.
- This is verified behaviour: a missing struct field is an execution error, and a missing map key silently renders empty unless `missingkey=error` is set. See [stdlib-html-template.md §4](stdlib-html-template.md#4-failure-modes-at-runtime).

### 3.4 Tooling: editor support

- **lucideer** ([comment](https://news.ycombinator.com/item?id=40490938)) noted incomplete syntax highlighting for `html/template`.
- **rickette** ([comment](https://news.ycombinator.com/item?id=40492481)) uses a double suffix: *"index.go.html… Most tools will recognize these as HTML… in JetBrains IDEs you can assign the 'GoTemplate' language to .go.html"*.
- templ has the opposite problem: it needs its own LSP and plugin. Zababa says *"editor integration and tooling weren't great"*, and pagoda says the plugin doesn't work in GoLand.

### 3.5 Type safety pulls people to templ

- **nzoschke** ([comment](https://news.ycombinator.com/item?id=48913636)): *"I pair it with a-h/templ for a bit more type safety on the template, components and partials."*
- **grahar64** ([comment](https://news.ycombinator.com/item?id=48915911)): the *"GoTH stack, Go Templ Htmx"*. They add sqlc for *"GoTHs or GHosT"*: *"once you buy into 'go generate ./...' as a build step you get so much."*
- **jackielii** ([comment](https://news.ycombinator.com/item?id=48918100)): *"Templ is great, but the ergonomics leaves many things to be desired. After writing a few large production applications in it. I decided to create gsx."*

### 3.6 htmx needs flexible fragments

- **yawaramin** ([comment](https://news.ycombinator.com/item?id=48915935)) recommends componentizing HTML in the backend *"much like React components"*. The reason: *"htmx requires a certain amount of flexibility… you need to be able to generate a certain piece of HTML markup and put a `<title>` tag immediately adjacent to it in some situations, and not in others… difficult with traditional string-based templating engines."*
- **sethops1** ([comment](https://news.ycombinator.com/item?id=48913966)) uses [cattlecloud/webtools/htmx](https://github.com/cattlecloud/webtools/tree/main/htmx). It chains a primary template response with out-of-band (OOB) fragments.
  - Their example passes `Fields: []any{count, thread}` and unpacks them with `{{$count := index . 0}}`.
  - That's a workaround for `html/template` accepting only one data argument.

### 3.7 The `Clone()` debate: stdlib template organisation is unclear

Alex Edwards' renderer clones a shared base+partials set on every request, then parses the page file into the clone.

- **akazantsev** ([comment](https://news.ycombinator.com/item?id=48917295)): *"You do not have to. It's a self-sustained injury by the author. You can compile the templates once and just execute them."*
- **alexedwards** ([comment](https://news.ycombinator.com/item?id=48917589)): *"The main reason for the clone operation is convenience."* Cloning lets each page redefine shared block names, such as `page:title`, without collisions.

Experienced Go developers still disagree on how to structure a template set. An adapter can settle this with a convention. Parse and clone per page at **startup**, not per request; see [stdlib-html-template.md §3](stdlib-html-template.md#3-organising-a-template-set).

### 3.8 htmx scepticism, and Datastar as the challenger

- **overflowy** ([1](https://news.ycombinator.com/item?id=48914423), [2](https://news.ycombinator.com/item?id=48918039)): *"it feels like the complexity of the codebase is growing at a 2:1 rate compared to the app itself."* Their examples are rich data grids and pending/error/retry states.
- **localhostinger** ([comment](https://news.ycombinator.com/item?id=48917022)): in teams, *"Lots of 'this is not a serious technology' kind of arguments."*
- **devnull3** ([comment](https://news.ycombinator.com/item?id=48917272)): *"It is staggering how Datastar is not more popular."*
- **nobleach** ([comment](https://news.ycombinator.com/item?id=48921060)): *"I think people see the 'pricing' and are turned off. The base is completely free!"* A downside they name is *"needing to have a template type that supports streaming."*
- **yawaramin** ([comment](https://news.ycombinator.com/item?id=48929027)) prefers htmx because *"Datastar doesn't support progressive enhancement"*, and uses `hx-boost` on real links and forms.

## 4. Synthesis

1. **Stdlib is the default, and people want it to work well.** Many Go developers pick `html/template` because it is built-in and escapes by default. They'd rather keep it than adopt a DSL or a build step.
2. **The pain is concrete and repeatable:**
   - Composing components with multiple arguments.
   - Organising layouts, pages and partials.
   - Errors that only appear at runtime.
   - Missing helper funcs.
   - Weak editor support.
   - Rendering fragments and OOB for htmx.

   Every complaint above is an ergonomics or tooling problem, not a missing capability.
3. **templ is the main alternative for type safety.** Its costs are a CLI, codegen, and an editor plugin, and those costs drive a visible minority back to stdlib or to gomponents. Newer entrants (gsx, gox) show the space isn't settled.
4. **htmx is the default interactivity layer** for Go server-rendered apps. Datastar is the energetic challenger. Its SSE-and-signals model is different enough that an htmx adapter shouldn't try to cover it, though a later SSE bridge could serve both (see [adapter-design-implications.md](adapter-design-implications.md)).
5. **Opportunity (inference):** a package that keeps `html/template` but removes the papercuts would serve the "stdlib is good enough" group, which seems to be the largest. That means:
   - Conventions for the template set.
   - Per-page clones built at startup.
   - Fragment and OOB rendering.
   - Safe funcs for htmx attributes.
   - Startup render checks.
   - Version-aware htmx request and response helpers.

   It shouldn't lock out templ or gomponents users; a one-method `Component` interface covers both.
