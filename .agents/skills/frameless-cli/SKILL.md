---
name: frameless-cli
description: Build and unit test Go command-line adapters with go.llib.dev/frameless/pkg/cli. Covers struct-tag inputs (flag, arg, env, default, enum, required, sep, min/max/range/length/regexp), ServeMux subcommands, help text and exit codes, and httptest-style in-process tests with cli.ResponseRecorder and cli.Request instead of exec-ing a built binary or swapping os.Args/os.Stdout. Use when writing or changing a CLI entrypoint (main, cmd/), adding commands, flags, positional args or env config, replacing flag/os.Args/cobra/urfave code in a frameless project, testing CLI output, exit codes or stdin handling without a shell, or changing pkg/cli itself. Applies to any Go project that imports go.llib.dev/frameless.
compatibility: Go projects importing go.llib.dev/frameless/pkg/cli.
license: MIT (matches go.llib.dev/frameless)
metadata:
  project: go.llib.dev/frameless
  package: pkg/cli
  scope: Global
---

# Frameless CLI

`go.llib.dev/frameless/pkg/cli` is `net/http` for command-line apps:

- A command is a `cli.Handler` with `ServeCLI(w, r)`.
- Inputs are declared as struct tags.
- `cli.ResponseRecorder` plays the role of `httptest.ResponseRecorder`.

Test the CLI adapter in-process, the same way you test an HTTP handler. You need no built binary, no `exec.Command`, no shell quoting and no `PATH`, so the same test runs on Linux, macOS and Windows.

## Stop signs

If you are about to write any of these, use `pkg/cli` instead:

- `exec.Command` of `go run`, `go build` or a compiled binary in a test, just to check output or the exit code
- swapping `os.Args`, `os.Stdout`, `os.Stderr` or `os.Stdin` in tests, or capturing output with `os.Pipe`
- `flag.Parse()`, `flag.CommandLine`, package-level flag variables, a `switch os.Args[1]`, or hand-written `strings.HasPrefix(arg, "--")` parsing
- `os.Getenv` inside a handler to read configuration that is really user input
- `os.Exit`, `log.Fatal` or `panic` in a command to signal failure (only `cli.Main` exits)
- `fmt.Println` or `os.Stdout`/`os.Stdin` inside a handler instead of `w` and `r.Body`

Do not add `spf13/cobra`, `urfave/cli`, `alecthomas/kong` or similar to a project that already depends on frameless.

## The httptest analogy

| `net/http` | `pkg/cli` |
| --- | --- |
| `http.Handler`, `ServeHTTP(w, r)` | `cli.Handler`, `ServeCLI(w cli.ResponseWriter, r *cli.Request)` |
| `http.HandlerFunc` | `cli.HandlerFunc` |
| `http.ServeMux`, `Handle(pattern, h)` | `cli.ServeMux`, `Handle("cmd", h)`, `Handle("sub cmd", h)`, `Sub("sub")` |
| path, query, headers | `arg`, `flag`, `env` struct tags, bound before the handler runs |
| request body | `r.Body` (stdin) |
| `r.Context()`, `r.WithContext(ctx)` | same (`Context()` defaults to `context.Background()`) |
| `w.WriteHeader(code)` | `w.ExitCode(code)` |
| `http.Error(w, msg, code)` | `cli.Error(w, msg, code)` (writes to stderr) |
| `http.ListenAndServe` | `cli.Main(ctx, h)` |
| `httptest.NewRecorder()` | `&cli.ResponseRecorder{}` (zero value is ready) |
| `rec.Code`, `rec.Body` | `rr.Code`, `rr.Out` (stdout), `rr.Err` (stderr), `rr.CombinedOutput()` |
| `httptest.NewRequest(...)` | `(&cli.Request{Args: args, Body: stdin}).WithContext(ctx)` |
| `h.ServeHTTP(rec, req)` | `cli.ServeCLI(h, rr, req)` |

One important difference: **`h.ServeCLI(rr, req)` does not parse.** The package-level `cli.ServeCLI(h, w, r)` is the full pipeline:

1. parse flags, args and env
2. apply defaults and validate
3. answer `-h`/`-help`/`--help`
4. reject bad input with exit code 2
5. otherwise call the handler on a parsed copy

A `*cli.ServeMux` routes first, then runs the same pipeline for the matched command. `cli.Main` builds a request from `os.Args[1:]` and `os.Stdin`, cancels the context on SIGINT, SIGHUP or SIGTERM, sends the frameless logger to stderr, then calls `cli.ServeCLI` and `os.Exit(code)`.

## Anatomy of a CLI adapter

```go
package greetcli

type Greeter interface { // the domain port the adapter delegates to
	Greet(ctx context.Context, lang, name string) (string, error)
}

func NewMux(g Greeter) *cli.ServeMux {
	var mux cli.ServeMux
	mux.WithSummary("greet people")
	mux.Handle("greet", GreetCommand{Greeter: g})
	mux.Sub("util").WithSummary("utility commands").Handle("echo", EchoCommand{})
	return &mux
}

type GreetCommand struct {
	Shout bool   `flag:"shout,s" desc:"print the greeting in upper case"`
	Lang  string `flag:"lang" env:"GREET_LANG" default:"en" enum:"en,hu," desc:"greeting language"`
	Name  string `arg:"0" desc:"who to greet"`

	// Greeter is a dependency, not user input: keep cli tags off it and off its fields.
	Greeter Greeter
}

func (GreetCommand) Summary() string { return "greet someone" } // listed by the mux help

func (cmd GreetCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	msg, err := cmd.Greeter.Greet(r.Context(), cmd.Lang, cmd.Name)
	if err != nil {
		cli.Error(w, err.Error(), cli.ExitCodeError)
		return // cli.Error does not stop the handler
	}
	if cmd.Shout {
		msg = strings.ToUpper(msg)
	}
	_, _ = fmt.Fprintln(w, msg)
}
```

```go
// cmd/greet/main.go: wiring only, nothing to test here
func main() {
	cli.Main(context.Background(), greetcli.NewMux(greeting.NewService()))
}
```

Rules:

- Keep `main` thin. Put commands and a `NewMux(deps...)` constructor in an importable package, so tests run the exact routing table production uses.
- Use value types with value receivers for handlers. The pipeline parses into a copy, so every run starts from the registered value. Pointer handlers (`&Cmd{}`) work, but parsing writes into the shared instance and state leaks between runs.
- Inject dependencies as exported, **untagged** fields (ports/interfaces) when you register the command.
  - The parser walks exported nested and embedded structs, pointers and interface values, including dependency values.
  - A tagged field inside a dependency becomes CLI input. An `env`-only field there even becomes required and can fail every run with exit code 2.
  - Unexported fields are ignored.
- Write results to `w`. Report failures with `cli.Error(w, msg, cli.ExitCodeError)` and then `return`. On success you don't need to call `ExitCode`, because `0` is the default.
- For warnings that shouldn't fail the run, write to stderr through `cli.ErrorWriter`:

  ```go
  if ew, ok := w.(cli.ErrorWriter); ok {
  	_, _ = fmt.Fprintln(ew.Stderr(), "warning: ...")
  }
  ```

- Pass `r.Context()` to every downstream call, and read stdin only from `r.Body`.
- Help hooks:
  - `Summary() string` (`cli.HelpSummary`) is shown in the mux command list.
  - `Description() string` (`cli.HelpDescription`) is shown in the command's help.
  - `Usage(pattern string) (string, error)` (`cli.HelpUsage`) replaces the generated usage entirely.
  - `cli.Usage(h, "cmd")` renders usage for a handler.
- For tabular output, use `cli.FPrintTable(w, rows, cli.TableRowPrefix("  "), cli.TablePadding(2))`.
- Use `cli.ResponseWriter` and `cli.ServeMux`. The names `cli.Response` and `cli.Mux` are deprecated, even though the package README still uses them.

## Declaring inputs

| Tag | Meaning |
| --- | --- |
| `flag:"name,n"` | Flag with aliases (leading dashes in the tag are ignored). `-n v`, `-n=v` and `--name v` are equivalent. A bool flag on its own means `true`. Optional by default. |
| `arg:"0"` | Positional argument by index. Indexes must be sequential. **Required by default.** |
| `arg:"1:"`, `arg:":"`, `arg:"1:3"` | Variadic or ranged positionals. Needs a slice field; a non-slice panics. |
| `env:"APP_X,X"` | Environment variable(s), first match wins. An env-only field is **required by default**. |
| `default:"v"` | Default value; makes the field optional. |
| `required:"true"`, `opt:"true"`/`optional:"true"` | Override required/optional. |
| `desc:"..."`/`description:"..."` | Text for the generated help. |
| `sep:","`/`separator:","` | Splits a value into slice elements. |
| `enum:"a,b"` | Allowed values; listed in the error message. A trailing punctuation character sets an explicit separator and keeps values verbatim: `enum:"a,b,"`, `enum:"x|y|"`. |
| `min`, `max`, `range:"0..100,200.."`, `length:"1..32"`/`len`, `char:"a-z"`, `regexp:"^v\\d+$"`/`rgx`/`match` | Validation from `pkg/validate`. A failure is exit code 2. |

- Precedence: flag > env > `default` > the value pre-set in the registered struct.
  - A `default` tag **overwrites** a pre-set value.
  - A pre-set value survives, and satisfies `required`, only on a field with no `default` when no flag or env is given.
  - Don't use tagged fields to inject configuration; use untagged fields.
- Supported field types:
  - `string`, `bool`, integers, unsigned integers, floats
  - `time.Duration` and `url.URL`
  - pointers
  - slices: use `sep`, a JSON array value (`["a","b"]`), or repeat the flag
  - maps, as a JSON value
  - any `encoding.TextUnmarshaler`, and types registered with `convkit.Register`
- `time.Time` fails with "missing TimeLayout ParseOption". Wrap it:

  ```go
  type Date struct{ time.Time }

  func (d *Date) UnmarshalText(text []byte) error {
  	t, err := time.Parse(time.DateOnly, string(text))
  	if err != nil {
  		return err
  	}
  	d.Time = t
  	return nil
  }
  ```

- Share flags across commands by embedding a struct:

  ```go
  type CommonFlags struct {
  	Verbose bool   `flag:"verbose,v"`
  	Format  string `flag:"format" default:"text" enum:"text,json,"`
  }

  type ReportCommand struct {
  	CommonFlags
  	Since Date     `flag:"since" desc:"YYYY-MM-DD"`
  	Tags  []string `flag:"tag" sep:","` // --tag a,b --tag c  →  [a b c]
  	Files []string `arg:"0:"`
  }
  ```

- **Flags must come before positional arguments.** Once the first positional is seen, nothing after it is parsed as a flag. `greet Ada -s` binds `Name = "Ada"` and silently drops `-s`, with no error. Put `--` before positionals that start with `-` (`report -v -- -dash.txt b.txt`).

## Exit codes and output streams

| Outcome | Code | Stream |
| --- | --- | --- |
| Handler succeeds | `cli.ExitCodeOK` (0) | handler's writes go to stdout |
| `cli.Error(w, msg, cli.ExitCodeError)` | 1 | `msg` + newline on stderr |
| `-h`, `-help`, `--help` on a command | 0 | stdout: usage, options, arguments; handler **not** called |
| Bad input: parse or validation error, enum mismatch, missing required arg or env, unknown flag | `cli.ExitCodeBadRequest` (2) | stderr: usage + `Invalid input:` + the offending flag/env/arg + cause (enum errors list accepted values); handler **not** called |
| Mux without a command, or with an unknown command | 2 | stderr: usage + command list |
| `-h` on a mux | 0 | stdout: command list, ` - name: Summary()` |

## Testing a CLI adapter

Pick the level that matches what you are testing:

| What | How | Parsing, defaults, validation, help? |
| --- | --- | --- |
| Handler logic with inputs already bound | `cmd.ServeCLI(rr, &cli.Request{...})`, with every field set in the struct literal | No. Set fields that have a `default` tag yourself. |
| Flags, args, env, defaults, validation, routing, help, exit codes | `cli.ServeCLI(mux, rr, req)` with `NewMux(fakes...)` | Yes, the same pipeline `cli.Main` runs |
| Binding only | `err := cli.ConfigureHandler(&cmd, req)`, then assert fields | Parsing and defaults; no help output, handler not called |

The full-pipeline act, in `go.llib.dev/testcase` spec style (see the `testcase-spec` skill):

```go
func TestNewMux(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		greeter = let.Var(s, func(t *testcase.T) *FakeGreeter { return &FakeGreeter{} })
		subject = let.Var(s, func(t *testcase.T) *cli.ServeMux { return greetcli.NewMux(greeter.Get(t)) })
		ctx     = let.Context(s)
		stdin   = let.VarOf(s, "")
	)

	s.Before(func(t *testcase.T) {
		testcase.UnsetEnv(t, "GREET_LANG") // env is process-global: pin it
	})

	serve := func(t *testcase.T, args ...string) *cli.ResponseRecorder {
		rr := &cli.ResponseRecorder{}
		req := &cli.Request{Args: slices.Clone(args), Body: strings.NewReader(stdin.Get(t))}
		t.OnFail(func() {
			t.Logf("args: %q\ncode: %d\nstdout:\n%s\nstderr:\n%s", args, rr.Code, rr.Out.String(), rr.Err.String())
		})
		cli.ServeCLI(subject.Get(t), rr, req.WithContext(ctx.Get(t)))
		return rr
	}

	s.Describe("greet", func(s *testcase.Spec) {
		var (
			name = let.Var(s, func(t *testcase.T) string { return t.Random.StringNC(8, random.CharsetAlpha()) })
			args = let.Var(s, func(t *testcase.T) []string { return []string{name.Get(t)} })
		)
		act := func(t *testcase.T) *cli.ResponseRecorder {
			return serve(t, append([]string{"greet"}, args.Get(t)...)...)
		}

		s.Then("it greets the named person in the default language", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, rr.Code, cli.ExitCodeOK)
			assert.Equal(t, rr.Out.String(), "hello "+name.Get(t)+"\n")
			assert.Empty(t, rr.Err.String())
			assert.Equal(t, greeter.Get(t).Lang, "en")
		})

		s.When("--lang is not an accepted value", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string { return []string{"--lang", "xx", name.Get(t)} })

			s.Then("it is rejected as a bad request without calling the handler", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
				assert.Contains(t, rr.Err.String(), "hu") // accepted values are listed
				assert.Equal(t, greeter.Get(t).Calls, 0)
			})
		})
	})
}
```

The complete, verified adapter and spec, including direct handler tests, `ConfigureHandler`, env precedence, cancellation, help, stdin and the root mux, is in [references/worked-example.md](references/worked-example.md).

Cover at least these scenarios per command:

| Scenario | Arrange | Assert |
| --- | --- | --- |
| Happy path | minimal valid args | code 0, exact stdout, empty stderr, fake got the bound values and defaults |
| Each flag and alias | `-s` / `--shout` | the behaviour it switches |
| Env input | `testcase.SetEnv` | value reaches the handler; a flag beats env |
| Invalid input | bad enum, out-of-range value, missing arg | code 2, message on stderr, fake **not** called |
| Dependency failure | fake returns an error | code 1, stderr message, nothing on stdout |
| Cancellation | cancelled `ctx` | the context reaches the dependency |
| Help | `-help` | code 0, stdout mentions the flags, fake not called |
| Stdin | `Body: strings.NewReader(...)` | output derived from input |
| Mux | `-h`, unknown command | command list with summaries; code 2 on stderr for unknown |

### Portability: what still depends on the host

The pipeline is pure Go and runs in-process. Two details of framework-generated text still come from the host:

- Help, usage and error text use the OS line separator (`\r\n` on Windows). Handler output written with `fmt.Fprintln` always uses `\n`.
- The `Usage:` line includes the test binary's name (`greetcli.test`, or `greetcli.test.exe` on Windows).

So compare handler output exactly, but match framework text loosely: `assert.Contains`, `assert.MatchRegexp(t, rr.Err.String(), `Usage: \S+ greet`)`, or normalise with `strings.ReplaceAll(s, "\r\n", "\n")`. Prefer asserting exit codes and fake calls over help wording.

### Test rules

- Use fakes for dependencies, not mocks. Pass them as pointers, so the value-copied handler and the test share state. On bad input, assert that the fake was **not** called.
- Build a fresh `*cli.Request` for every run, because the mux and the parser rewrite `r.Args`. Clone the args you log.
- Set `Body` when the command reads stdin. A zero `cli.Request` has a nil `Body`; only `cli.Main` uses `os.Stdin`.
- Env is read with `os.LookupEnv`, which is process-global. Pin every variable a command reads with `testcase.SetEnv`/`testcase.UnsetEnv` (or `t.Setenv`), usually unset in a root `Before`, so the developer's shell or CI can't change outcomes. Tests that set env can't use `t.Parallel()`.
- `rr.Code == 0` is also the zero value. Pair it with an output or fake assertion.
- `rr.CombinedOutput()` is stdout followed by stderr, not interleaved in time.
- With testcase, `LetValue`/`let.VarOf` refuse slices and maps. Use `let.Var`/`Let` for `[]string` args.
- Use alphabetic random values for positionals (`t.Random.StringNC(n, random.CharsetAlpha())`). A random string starting with `-` would be parsed as a flag.
- Don't test `main`. If `main` only wires dependencies and calls `cli.Main`, there is nothing left to test there.

## Changing `pkg/cli` itself

- Run `go test ./pkg/cli/...`. The specs in `cli_test.go`, `help_test.go` and `table_test.go` use `go.llib.dev/testcase`; follow that style.
- `cli.ServeCLI` and `cli.ConfigureHandler` share the binding logic. A new tag or field type must behave the same in both, show up in the generated help, and report bad values as exit code 2 with `Invalid input:`.
- Framework text goes through `lineSeparator` and `execName()`. Tests in the package must not hard-code `\n` or the binary name for that text.
- Keep `pkg/cli/README.md` and `pkg/cli/example/{direct,mux}` in sync with API changes. The README still uses the deprecated `cli.Response`/`cli.Mux` names.
- Known rough edges, all candidates for upstream fixes:
  - flags after the first positional are silently ignored instead of rejected;
  - `default` overwrites values pre-set on the registered handler;
  - `time.Time` needs a wrapper type;
  - tags inside dependency structs are processed;
  - `ResponseRecorder` and `StdResponse` expose a misspelled `Stdeout()` method.

## Reference

- `pkg/cli/README.md`: overview, HTTP↔CLI terminology, tag examples
- `pkg/cli/cli.go`: `Handler`, `Request`, `ResponseWriter`, `ServeMux`, `ServeCLI`, `Main`, `ConfigureHandler`, `ResponseRecorder`, exit codes, tag handling
- `pkg/cli/help.go`, `pkg/cli/doc.go`: `Usage`, `HelpUsage`, `HelpSummary`, `HelpDescription`
- `pkg/cli/table.go`: `FPrintTable`, `TableRowPrefix`, `TablePadding`
- `pkg/cli/deprecated.go`: old names
- `pkg/cli/example/direct`, `pkg/cli/example/mux`: runnable examples
- `pkg/cli/*_test.go`: behaviour specs
- [references/worked-example.md](references/worked-example.md): a complete adapter with its spec

## Checklist

- [ ] No `exec.Command` of the binary, no `os.Args`/`os.Stdout`/`os.Stdin` swapping, and no `flag` package globals. Tests go through `cli.ServeCLI` with a `cli.ResponseRecorder`.
- [ ] `main` only wires dependencies and calls `cli.Main`. Commands and `NewMux(deps...)` live in an importable package.
- [ ] Handlers are value types. Dependencies are untagged exported fields. Inputs are tagged fields with `desc`.
- [ ] Handlers write to `w`, read stdin from `r.Body`, pass `r.Context()` downstream, and `return` after `cli.Error`.
- [ ] Every command has a `Summary()`. Enums, ranges and required inputs are declared as tags, not checked by hand.
- [ ] Tests cover the happy path, each flag and env input, invalid input (code 2, handler not called), dependency failure (code 1), cancellation and help.
- [ ] Env is pinned in tests, a fresh `cli.Request` is built for every run, and framework-generated text is matched loosely (line separators, binary name).
- [ ] Positional arguments come after flags in test args, with `--` before values that start with `-`.
