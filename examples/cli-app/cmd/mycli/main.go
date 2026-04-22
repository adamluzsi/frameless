package main

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/logging"
)

func main() {
	var mux cli.ServeMux
	mux.WithDescription("Example cli app to showcase how the same pattern used for building HTTP web apps\ncan used for command line utilities")

	mux.Handle("foo", FooCommand{})
	mux.Handle("bar", BarCommand{})
	mux.Handle("baz", BazCommand{})

	var submux = mux.Sub("sub").
		WithSummary("xyz sub topic of this cli").
		WithDescription("detailed description about this sub command namespace")
	submux.Handle("cmd", SubCommand{})

	ctx := context.Background()
	ctx = logging.ContextWith(ctx, logging.Field("app", "mycli"))
	cli.Main(ctx, &mux)
}

type FooCommand struct {
	Foo string `desc:"foo input" flag:"foo" opt:"T"`
	Bar bool   `desc:"bar input" flag:"bar,b" default:"false"`
	Baz int    `desc:"baz input" arg:"0" opt:"F"`
}

func (cmd FooCommand) Summary() string { return "doing foo stuff" }

func (cmd FooCommand) Description() string { return "foo foo so foo foo fooer foo" }

func (cmd FooCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
}

type BarCommand struct {
	ArgA string `arg:"1"`
	ArgB string `arg:"2" opt:"true"`
	ArgC string `arg:"3" opt:"true"`
}

func (cmd BarCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {}

type BazCommand struct {
	Arg string `arg:"1"`
	Opt string `arg:"2" env:"X_OPT" opt:"true"`

	SomeRandomConfigWithEnvConfiguration // embedding config is supported
}

func (cmd BazCommand) Summary() string { return "doing baz stuff" }

func (cmd BazCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	if cmd.Arg != "hello" {
		w.ExitCode(cli.ExitCodeBadRequest)
		fmt.Fprintf(w, "%s\n", `first input argument must be "hello"`)
		return
	}

	fmt.Fprintf(w, "World!\n")
}

type SomeRandomConfigWithEnvConfiguration struct {
	Foo string  `env:"FOO" opt:"T"`
	Bar int     `env:"BAR" opt:"T"`
	Baz float64 `env:"BAZ" opt:"T"`
	Qux float64 `env:"QUX" opt:"T"`
}

type SubCommand struct{}

func (cmd SubCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {}
