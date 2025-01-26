package main

import (
	"testing"

	"go.llib.dev/frameless/pkg/cli"
)

func TestTestCommand(t *testing.T) {
	cmd := TestCommand{
		BoolFlag:   true,
		StringFlag: "foo",
		StringArg:  "bar",
		IntArg:     42,
	}

	rr := &cli.ResponseRecorder{}
	req := &cli.Request{Args: []string{}}

	cmd.ServeCLI(rr, req)
}
