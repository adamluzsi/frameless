# CLI

The `cli` package meant to give you tooling in building command line interface applications.

It also purposefully use an package convention that makes it familiar to the HTTP package's interface.

Terminology similarities between HTTP and CLI:

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

To create a CLI command, you simply need to design a structure that implements the cli.Handler interface.

This structure can list all its options and arguments, which are automatically parsed and displayed in the command’s documentation when help is requested.

Fields in the structure represent the command’s dependencies.

You can use specific tags to define how these fields should behave:

- `flag`: Marks the field as a CLI option for your command.
- `arg`: Indicates that the field is expected as a positional argument at a specific index.

Additional Tags for Further Specification

You can combine these tags to refine your command:

- `desc`: Provides a description of the given flag or argument.
- `default`: Sets a default value if the user does not supply one.
- `required`: Marks the field as mandatory, ensuring the user provides it.

```go
type TestCommand struct {
	BoolFlag   bool   `flag:"bool" desc:"a bool flag"`
	StringFlag string `flag:"str" default:"foo"`

	StringArg string `arg:"0" required:"true"`
	IntArg    int    `arg:"1" default:"42"`
}

func (cmd TestCommand) ServeCLI(w cli.Response, r *cli.Request) {
	fmt.Fprintln(w, pp.Format(cmd))
}
```

```txt
Usage: direct [OPTION]... [StringArg] [IntArg]

Options:
  -bool=[bool]: a bool flag
  -str=[string] (Default: foo)

Arguments:
  StringArg [string]
  IntArg [int] (Default: 42)

flag: help requested
exit status 2
```

## Example

### direct

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
	BoolFlag   bool   `flag:"bool" desc:"a bool flag"`
	StringFlag string `flag:"str" default:"foo"`

	StringArg string `arg:"0" required:"true"`
	IntArg    int    `arg:"1" default:"42"`
}

func (cmd TestCommand) ServeCLI(w cli.Response, r *cli.Request) {
	fmt.Fprintln(w, pp.Format(cmd))
}
```

### multi command

Using the `cli.Mux`, you can register commands and sub commands in your app.

```go
package main

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase/pp"
)

func main() {
	var m cli.Mux
	m.Handle("test", TestCommand{})
	m.Handle("foo", FooCommand{})
	m.Handle("baz", BazCommand{})

	sub := m.Sub("sub")
	sub.Handle("bar", BarCommand{})

	cli.Main(context.Background(), m)
}

type TestCommand struct {
	BoolFlag   bool   `flag:"bool"`
	StringFlag string `flag:"str" default:"foo"`

	StringArg string `arg:"0" default:"foo"`
	IntArg    int    `arg:"1" default:"42"`
}

func (cmd TestCommand) ServeCLI(w cli.Response, r *cli.Request) {
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

func (cmd FooCommand) ServeCLI(w cli.Response, r *cli.Request) {
	fmt.Fprintf(w, "%#v\n", cmd)
}

type BarCommand struct{}

func (cmd BarCommand) ServeCLI(w cli.Response, r *cli.Request) {
	fmt.Fprintln(w, "bar")
}

type BazCommand struct {
	First  string `arg:"0"`
	Second string `arg:"1"`
}

func (cmd BazCommand) ServeCLI(w cli.Response, r *cli.Request) {
	fmt.Fprintln(w, "baz")
}


```