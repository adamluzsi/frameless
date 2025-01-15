package clikit_test

import (
	"context"

	"go.llib.dev/frameless/pkg/clikit"
)

func Example() {
	var cli clikit.Router
	cli.Command("foo", FooCommand{})
	subCMD := cli.Sub("sub")
	subCMD.Command("subcmd", SubCommand{})

	cli.Main(context.Background())
}

type FooCommandArgs struct {
	VeryDescriptiveArgumentName string
	NotSoDescriptiveArgName     bool `desc:"but the description here makes it descriptive"`
}

type FooCommand struct {
	A string `flag:"the-a,a" default:"val"   desc:"this is flag A"`
	B bool   `flag:"the-b,b" default:"true"` // missing description
	C int    `flag:"c" required:"true"       desc:"this is flag C, not B"`
	D string `flag:"d" enum:"FOO,BAR,BAZ,"   desc:"this flag is an enum"`

	// Dependency is a dependency of the FooCommand, which is populated though traditional dependency injection.
	Dependency string
}

func (cmd FooCommand) Help(ctx context.Context) (string, error) {
	return "Foo Documentation", nil
}

func (cmd FooCommand) Run(ctx context.Context, arg1 string, arg2 bool) error {

	return nil
}

type SubCommand struct{}

func (cmd SubCommand) Run(ctx context.Context) error {

	return nil
}
