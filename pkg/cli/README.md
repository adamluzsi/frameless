# CLI

The `cli` package is designed to help you build command-line interface applications easily.

It follows a familiar structure, similar to the `net/http` stdlib package, so developers can quickly adapt using their existing knowledge.

The `cli` package also aim to simplify testing of CLI commands,
allowing you to test cleanly without needing to create stubs, mocks, or fake shared values in your application.

**Terminology similarities between HTTP and CLI**:

| HTTP                    | CLI                       | desc                                                                               |
| ----------------------- | ------------------------- | ---------------------------------------------------------------------------------- |
| request path            | command name in args      | defines what handler/command the caller wishes to reach                            |
| request path parameters | command arguments in args | endpoint specific parameters                                                       |
| request body            | STDIN                     | contains the user input data payload                                               |
| response body           | STDOUT                    | the channel in which the application replies back to the caller                    |
| request query string    | flags                     | interaction related meta data or modifiers that expect the affect to be altered    |
| request headers         | env variables             | ~same~                                                                             |
| status code             | exit code                 | code that notifies the caller if request succeeded or failed                       |
| request cancellation    | OS Signal interrupt       | an idiom to notify the software that the response no longer expected by the caller |

## Quick Start

To create a CLI command, you simply need to design a structure that implements the `cli.Handler` interface.

This structure can list all its options and arguments, which are automatically parsed and displayed in the command’s documentation when help is requested.

Fields in the structure represent the command’s inputs and dependencies.

You can use specific tags to define how these fields should behave:

- `flag`: Marks the field as a CLI option for your command.
- `arg`: Indicates that the field is expected as a positional argument at a specific index.
- `env`: The environment variable key that can be used as an alternative way to a flag configure the CLI command.
  - `flag` and an `env` can coexist for a field, where `flag` will be prioritised over `env`.

Additional Tags for Further Specification

You can combine these tags to refine your command:

- `desc`: Provides a description of the given flag or argument.
- `default`: Sets a default value if the user does not supply one. See [Default values](#default-values).
- `required`: Marks the field as mandatory, ensuring the user provides it.
- `enum`: Set a list of enumerator value for the field, which will define what values the CLI accept for a given input.
  - the error message for an invalid input will list the acceptable values.
- `separator` (or `sep`): Sets the separator that splits a value into the elements of a slice field.
  - Without a `separator` tag, the value is split on commas (`,`).
  - A JSON array value, like `["a,b","c"]`, is parsed as JSON, with or without a `separator` tag,
    so its elements can contain the separator.
  - Escape the separator within an element with a backslash, e.g. `a\,b,c` is `["a,b" "c"]`.
  - These are the same rules that `frameless/pkg/env` follows.
- `layout`: Sets the layout of a `time.Time` input, in the format of `time.Parse`, e.g. `layout:"2006-01-02"`.
  - Without a `layout` tag, a `time.Time` input is expected in `time.RFC3339` format.
  - The `-help` documentation shows the expected layout.
- `opaque` (alias `norecurse`): Stops the cli package from looking for inputs inside the field's value.
  - `opaque:""` and `opaque:"true"` turn it on, `opaque:"false"` turns it off.
  - The tag is shared with `frameless/pkg/env`.
  - See [Dependency injection](#dependency-injection).

```go
type TestCommand struct {
	BoolFlag   bool   `flag:"bool" env:"BOOLENVVAR" desc:"a bool flag"`
	StringFlag string `flag:"str" env:"STR_ENVVAR" default:"foo"`

	StringArg string `arg:"0" required:"true"`
	IntArg    int    `arg:"1" default:"42"`
}

func (cmd TestCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	fmt.Fprintln(w, pp.Format(cmd))
}
```

```txt
Usage: direct [OPTION]... StringArg IntArg

Options:
  -bool=[bool]: a bool flag (env: BOOLENVVAR)
  -str=[string] (env: STR_ENVVAR) (default: "foo")

Arguments:
  StringArg [string]
  IntArg [int] (Default: "42")

Environments:
  BOOLENVVAR [bool]
      a bool flag
      Substitute bool flag
  STR_ENVVAR [string]
       (default: "foo")
      Substitute str flag
```

### time.Time inputs

```go
type ReportCommand struct {
	Since time.Time `flag:"since" layout:"2006-01-02" default:"2000-01-01"`
	Until time.Time `flag:"until"` // RFC3339, e.g. 2006-01-02T15:04:05Z
}
```

The `default` tag value of a `time.Time` field is parsed with the same layout.

## Default values

A `default` tag value is used for an input when the user didn't provide it
and the field of the registered handler is still zero.
If you pre-set an input on the registered handler, the `default` tag doesn't overwrite it,
while the rest of the inputs still get their `default` tag values:

```go
cli.Main(context.Background(), FooCommand{
	A: "pre-set", // kept, instead of A's `default:"val"`
})
```

When the command runs, an input gets its value from the first of these that has one:

1. the flag or argument the user passed
2. the environment variable
3. the value pre-set on the registered handler
4. the `default` tag

A zero value can't be told apart from an unset one,
so an input pre-set to its zero value still gets its `default` tag value.
Think twice before you put `default:"true"` on a `bool` flag,
because you can't turn it off from code. Only the user can, with `-flag=false`.
Prefer a flag name that is off by default, like `no-color` instead of `color` with `default:"true"`.

To apply the `default` tag values without running the command,
for example when you call the handler's ServeCLI method directly in a test, use `cli.WithDefaults`.
It sets the `default` tag value of every zero input field and keeps the rest.

Range and variadic arguments (`arg:"0:2"`, `arg:"1:"`) follow the same order.
When the user passes none of their values, they get their environment variable, pre-set value, or `default` tag value,
which is split the same way as any other slice value:

```go
type CatCommand struct {
	Files []string `arg:"0:" default:"a.txt,b.txt"`
}
```

A required range or variadic argument that gets no value from any of these is a bad request.

## Struct fields as inputs

The inputs of a nested struct come from the tags of its fields.
When the struct field has a `flag`, `env` or `default` tag of its own, it takes a JSON object as well:

```go
type DBConfig struct {
	Host  string `flag:"db-host" default:"localhost"`
	Port  int    `flag:"db-port" default:"5432"`
	Token string `flag:"db-token" env:"APP_DB_TOKEN" required:"true"`
}

type ServeCommand struct {
	DB DBConfig `flag:"db" env:"APP_DB"` // -db '{"Host":"db.internal","Token":"secret"}'
}
```

The values are applied in this order, and a later one overwrites an earlier one:

1. the `default` tags of the inner fields
2. the `default` tag of the struct field, when it holds a JSON object
3. the flags, arguments and environment variables of the inner fields
4. the flag or environment variable of the struct field

So a default never overwrites an input, and the order of the flags doesn't matter:
`-db '{"Host":"a"}' -db-host b` and `-db-host b -db '{"Host":"a"}'` both end with the host `a`.
A JSON object is merged onto the field's value, so the inner fields it doesn't mention keep what they got.
With `-db '{"Port":6543}'`, the host stays `localhost`.
An invalid JSON value is a bad request.

The `required` and validation tags are checked once every input is applied.
A required inner field that got no input is satisfied
when the JSON object of its struct field, from its input or its `default` tag, gives it a non-empty value.
An input that the user gave satisfies `required`, even when it is empty, like `-db-token ""`.

Tag the struct field with `opaque:""` when its type's own tags must not become inputs.
The field then takes only the JSON object (see [Dependency injection](#dependency-injection)).

## Testing

Testing with `frameless/pkg/cli` is designed to be simple, and it works the same way as `net/http/httptest`.
Record the response with a `cli.ResponseRecorder`, which collects the exit code,
the standard output into `Out` (also available as an `io.Writer` with `Stdout()`)
and the standard error into `Err` (with `Stderr()`).

To test the handler logic alone, set up your command value based on your testing scenario and call its ServeCLI method.
This skips the input parsing, so you need to set every input yourself,
or start from `cli.WithDefaults(MyCommand{})` to get the `default` tag values.

```go
func TestMyCommand(t *testing.T) {
	cmd := MyCommand{ // setting up your flag/arg configuration
		BoolFlag:   true,
		StringFlag: "foo",
		StringArg:  "bar",
		IntArg:     42,
	}

	rr := &cli.ResponseRecorder{}
	req := &cli.Request{}

	cmd.ServeCLI(rr, req)

	if rr.Code != cli.ExitCodeOK {
		t.Fatalf("unexpected exit code: %d\n%s", rr.Code, rr.Err.String())
	}
}
```

To test the whole command, including flags, arguments, environment variables, defaults, validation, help and routing,
pass the request through `cli.ServeCLI`. This is the same pipeline that `cli.Main` runs.

```go
func TestMyCommand_input(t *testing.T) {
	rr := &cli.ResponseRecorder{}
	req := &cli.Request{Args: []string{"-bool", "-str", "foo", "bar", "42"}}

	cli.ServeCLI(MyCommand{}, rr, req)

	if rr.Code != cli.ExitCodeOK {
		t.Fatalf("unexpected exit code: %d\n%s", rr.Code, rr.Err.String())
	}
}
```

The text the framework writes, like the help, the usage and the input error reports, is the same on every OS.
It always uses `"\n"` line endings, and the program name in the usage has no `.exe` extension.

## Dependency injection

If your command/cli.Handler has its own dependencies,
you can simply pass the preconfigured command structure as the handler,
the dependencies won't be affected by argument parsing.
Keep the input tags off the dependency fields.

```go
func main() {
	cli.Main(context.Background(), CommandWithDependency{
		Dependency: "important dependency that I need as part of the ServeCLI call",
	})
}
```

The cli package looks for inputs in nested and embedded structs too, including the values behind pointers and interfaces.
This lets you share inputs between commands by embedding a struct with tagged fields.
It also means that a dependency whose type has `flag`, `arg` or `env` tags of its own would turn them into inputs,
like the command struct of another CLI framework, or a config type with different tag conventions.
Tag such a field with `opaque:""` (or its alias `norecurse:""`), and the cli package won't look inside its value:

```go
type ServeCommand struct {
	Port int `flag:"port" default:"8080"`

	// The tags of legacy.Command are not inputs, and the dependency is left as is.
	Legacy *legacy.Command `opaque:""`
	// Embedded fields work the same way.
	legacy.Options `opaque:""`

	// The field itself can still be an input, which takes a JSON value: -config '{"Name":"foo"}'
	Config Config `flag:"config" env:"APP_CONFIG" opaque:""`
}
```

An `opaque` tag value that is not a bool panics, so a mistyped tag can't go unnoticed.
`frameless/pkg/env` understands the same tag, so a type can be opaque for both packages.

## Example

### direct

From [example/direct/main.go](example/direct/main.go):

```go
package main

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase/pp"
)

func main() {
	cli.Main(context.Background(), TestCommand{})
}

type TestCommand struct {
	BoolFlag   bool   `flag:"bool" env:"BOOLENVVAR" desc:"a bool flag"`
	StringFlag string `flag:"str" env:"STR_ENVVAR" default:"foo"`

	StringArg string `arg:"0" required:"true"`
	IntArg    int    `arg:"1" default:"42"`
}

func (cmd TestCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	fmt.Fprintln(w, pp.Format(cmd))
}
```

### multi command setup

The `cli.ServeMux` allows you to dispatch the cli request between commands and sub-commands in your application.
A command can also be registered with its full path, e.g. `m.Handle("sub bar", BarCommand{})`.

From [example/mux/main.go](example/mux/main.go):

```go
package main

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase/pp"
)

func main() {
	var m cli.ServeMux
	m.WithDescription("my cli app")

	m.Handle("test", TestCommand{})
	m.Handle("foo", FooCommand{})
	m.Handle("baz", BazCommand{})

	sub := m.Sub("sub")
	sub.WithDescription("sub command")
	sub.Handle("bar", BarCommand{})

	cli.Main(context.Background(), &m)
}

type TestCommand struct {
	BoolFlag   bool   `flag:"bool"`
	StringFlag string `flag:"str" default:"foo"`

	StringArg string `arg:"0" default:"foo"`
	IntArg    int    `arg:"1" default:"42"`
}

func (cmd TestCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	fmt.Fprintln(w, pp.Format(cmd))
}

type FooCommand struct {
	A string `flag:"the-a,a" default:"val"   desc:"this is flag A"`
	B bool   `flag:"the-b,b" default:"true"` // missing description
	C int    `flag:"c" required:"true"       desc:"this is flag C, not B"`
	D string `flag:"d" enum:"FOO,BAR,BAZ,"   desc:"this flag is an enum"`

	Arg    string `arg:"0" desc:"something something"`
	OthArg int    `arg:"1" default:"42"`

	// Dependency is a dependency of the FooCommand, which is populated though traditional dependency injection.
	Dependency string
}

func (cmd FooCommand) Summary() string { return "foo command" }

func (cmd FooCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	fmt.Fprintf(w, "%#v\n", cmd)
}

type BarCommand struct{}

func (cmd BarCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	fmt.Fprintln(w, "bar")
}

type BazCommand struct {
	First  string `arg:"0"`
	Second string `arg:"1"`
}

func (cmd BazCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	fmt.Fprintln(w, "baz")
}
```
