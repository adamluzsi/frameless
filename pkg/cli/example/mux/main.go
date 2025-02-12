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

	cli.Main(context.Background(), &m)
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
