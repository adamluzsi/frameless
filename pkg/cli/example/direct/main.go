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
