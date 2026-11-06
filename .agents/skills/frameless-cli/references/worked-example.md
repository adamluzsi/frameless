# Worked example: a CLI adapter and its spec

This is a complete, verified example: a small greeting CLI with two commands, plus a `go.llib.dev/testcase` spec. It tests the adapter in-process through `cli.ResponseRecorder`, with no binary and no shell.

It shows:

- direct handler tests;
- binding-only tests with `cli.ConfigureHandler`;
- full-pipeline tests with `cli.ServeCLI` over the real `NewMux`, covering flags, env precedence, enum validation, missing arguments, cancellation, help, stdin and the root mux.

Replace `example.com/app` with your module path.

## `greetcli/greet.go`

```go
// Package greetcli is the CLI adapter of a small greeting app.
package greetcli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"go.llib.dev/frameless/pkg/cli"
)

// Greeter is the domain port the CLI adapter delegates to.
type Greeter interface {
	Greet(ctx context.Context, lang, name string) (string, error)
}

// NewMux wires every command with its dependencies.
// main and the tests both use it, so the tests exercise the real routing table.
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

func (GreetCommand) Summary() string { return "greet someone" }

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

type EchoCommand struct{}

func (EchoCommand) Summary() string { return "copy stdin to stdout" }

func (EchoCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	if _, err := io.Copy(w, r.Body); err != nil {
		cli.Error(w, err.Error(), cli.ExitCodeError)
	}
}
```

## `cmd/greet/main.go`

```go
package main

import (
	"context"

	"go.llib.dev/frameless/pkg/cli"

	"example.com/app/greetcli"
	"example.com/app/greeting"
)

func main() {
	cli.Main(context.Background(), greetcli.NewMux(greeting.NewService()))
}
```

## `greetcli/greet_test.go`

```go
package greetcli_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"

	"example.com/app/greetcli"
)

// FakeGreeter is a hand-written fake of the Greeter port that records how it was called.
type FakeGreeter struct {
	Err error

	Calls  int
	Lang   string
	Name   string
	CtxErr error
}

func (f *FakeGreeter) Greet(ctx context.Context, lang, name string) (string, error) {
	f.Calls++
	f.Lang, f.Name, f.CtxErr = lang, name, ctx.Err()
	if f.Err != nil {
		return "", f.Err
	}
	return "hello " + name, nil
}

// TestGreetCommand covers the handler's own logic and its input binding.
func TestGreetCommand(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		greeter = let.Var(s, func(t *testcase.T) *FakeGreeter {
			return &FakeGreeter{}
		})
		subject = let.Var(s, func(t *testcase.T) greetcli.GreetCommand {
			return greetcli.GreetCommand{
				Lang:    "en", // a direct ServeCLI call does no parsing, so tag defaults are not applied
				Name:    t.Random.StringNC(8, random.CharsetAlpha()),
				Greeter: greeter.Get(t),
			}
		})
	)

	s.Describe("#ServeCLI", func(s *testcase.Spec) {
		act := func(t *testcase.T) *cli.ResponseRecorder {
			rr := &cli.ResponseRecorder{}
			subject.Get(t).ServeCLI(rr, &cli.Request{})
			return rr
		}

		s.Then("it writes the greeting to stdout", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, rr.Code, cli.ExitCodeOK)
			assert.Equal(t, rr.Out.String(), "hello "+subject.Get(t).Name+"\n")
			assert.Empty(t, rr.Err.String())
			assert.Equal(t, greeter.Get(t).Lang, "en")
		})

		s.When("Shout is enabled", func(s *testcase.Spec) {
			subject.Let(s, func(t *testcase.T) greetcli.GreetCommand {
				cmd := subject.Super(t)
				cmd.Shout = true
				return cmd
			})

			s.Then("the greeting is upper-cased", func(t *testcase.T) {
				assert.Equal(t, act(t).Out.String(), strings.ToUpper("hello "+subject.Get(t).Name)+"\n")
			})
		})

		s.When("the greeter fails", func(s *testcase.Spec) {
			expErr := let.Error(s)
			greeter.Let(s, func(t *testcase.T) *FakeGreeter {
				return &FakeGreeter{Err: expErr.Get(t)}
			})

			s.Then("the error goes to stderr with the general error exit code", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeError)
				assert.Equal(t, rr.Err.String(), expErr.Get(t).Error()+"\n")
				assert.Empty(t, rr.Out.String())
			})
		})
	})

	s.Describe("input binding", func(s *testcase.Spec) {
		args := let.Var(s, func(t *testcase.T) []string {
			return []string{"-s", "Ada"}
		})

		act := func(t *testcase.T) (greetcli.GreetCommand, error) {
			var cmd greetcli.GreetCommand
			err := cli.ConfigureHandler(&cmd, &cli.Request{Args: args.Get(t)})
			return cmd, err
		}

		s.Before(func(t *testcase.T) {
			testcase.UnsetEnv(t, "GREET_LANG")
		})

		s.Then("flags, args and defaults are bound to the fields", func(t *testcase.T) {
			cmd, err := act(t)
			assert.NoError(t, err)
			assert.True(t, cmd.Shout)
			assert.Equal(t, cmd.Name, "Ada")
			assert.Equal(t, cmd.Lang, "en")
		})

		s.When("a flag comes after the positional argument", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"Ada", "-s"}
			})

			s.Then("the flag is silently not parsed", func(t *testcase.T) {
				cmd, err := act(t)
				assert.NoError(t, err)
				assert.False(t, cmd.Shout)
				assert.Equal(t, cmd.Name, "Ada")
			})
		})
	})
}

// TestNewMux runs the whole pipeline in-process: routing, flag/env/arg parsing,
// defaults, validation and help, as httptest does for an http.Handler.
func TestNewMux(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		greeter = let.Var(s, func(t *testcase.T) *FakeGreeter {
			return &FakeGreeter{}
		})
		subject = let.Var(s, func(t *testcase.T) *cli.ServeMux {
			return greetcli.NewMux(greeter.Get(t))
		})
		ctx   = let.Context(s)
		stdin = let.VarOf(s, "")
	)

	s.Before(func(t *testcase.T) {
		// env is read from the process: pin it so the developer's shell can't change the outcome
		testcase.UnsetEnv(t, "GREET_LANG")
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
			name = let.Var(s, func(t *testcase.T) string {
				return t.Random.StringNC(8, random.CharsetAlpha())
			})
			args = let.Var(s, func(t *testcase.T) []string {
				return []string{name.Get(t)}
			})
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
			assert.Equal(t, greeter.Get(t).Name, name.Get(t))
		})

		s.When("the shout flag is given", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-s", name.Get(t)} // flags must come before positional args
			})

			s.Then("the greeting is upper-cased", func(t *testcase.T) {
				assert.Equal(t, act(t).Out.String(), strings.ToUpper("hello "+name.Get(t))+"\n")
			})
		})

		s.When("GREET_LANG is set", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, "GREET_LANG", "hu")
			})

			s.Then("the language comes from the environment", func(t *testcase.T) {
				act(t)
				assert.Equal(t, greeter.Get(t).Lang, "hu")
			})

			s.And("the --lang flag is also given", func(s *testcase.Spec) {
				args.Let(s, func(t *testcase.T) []string {
					return []string{"--lang", "en", name.Get(t)}
				})

				s.Then("the flag wins over the environment", func(t *testcase.T) {
					act(t)
					assert.Equal(t, greeter.Get(t).Lang, "en")
				})
			})
		})

		s.When("--lang is not an accepted value", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"--lang", "xx", name.Get(t)}
			})

			s.Then("it is rejected as a bad request without calling the handler", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
				assert.Empty(t, rr.Out.String())
				assert.Contains(t, rr.Err.String(), "en")
				assert.Contains(t, rr.Err.String(), "hu")
				assert.Equal(t, greeter.Get(t).Calls, 0)
			})
		})

		s.When("the name argument is missing", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string { return nil })

			s.Then("usage is printed to stderr with a bad request exit code", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
				assert.MatchRegexp(t, rr.Err.String(), `Usage: \S+ greet`) // the binary name differs per OS
				assert.Equal(t, greeter.Get(t).Calls, 0)
			})
		})

		s.When("the context is cancelled", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				c, cancel := context.WithCancel(ctx.Super(t))
				cancel()
				return c
			})

			s.Then("the cancellation reaches the dependency", func(t *testcase.T) {
				act(t)
				assert.ErrorIs(t, greeter.Get(t).CtxErr, context.Canceled)
			})
		})

		s.When("help is requested", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string { return []string{"-help"} })

			s.Then("the command help goes to stdout and the handler is not called", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeOK)
				assert.Contains(t, rr.Out.String(), "-lang")
				assert.Equal(t, greeter.Get(t).Calls, 0)
			})
		})
	})

	s.Describe("util echo", func(s *testcase.Spec) {
		stdin.Let(s, func(t *testcase.T) string { return t.Random.String() })

		act := func(t *testcase.T) *cli.ResponseRecorder {
			return serve(t, "util", "echo")
		}

		s.Then("stdin is copied to stdout", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, rr.Code, cli.ExitCodeOK)
			assert.Equal(t, rr.Out.String(), stdin.Get(t))
		})
	})

	s.Describe("root", func(s *testcase.Spec) {
		args := let.Var(s, func(t *testcase.T) []string {
			return []string{"-h"}
		})

		act := func(t *testcase.T) *cli.ResponseRecorder {
			return serve(t, args.Get(t)...)
		}

		s.Then("help lists the commands with their summaries", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, rr.Code, cli.ExitCodeOK)
			assert.Contains(t, rr.Out.String(), " - greet: greet someone")
		})

		s.When("the command is unknown", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string { return []string{"unknown"} })

			s.Then("it fails as a bad request with help on stderr", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
				assert.Contains(t, rr.Err.String(), "greet")
				assert.Equal(t, greeter.Get(t).Calls, 0)
			})
		})
	})
}
```

## What the bad-input output looks like

From a `ReportCommand` with an embedded `CommonFlags` (`Format string flag:"format" default:"text" enum:"text,json,"`), run with `--format xml`. The `Usage:` line shows the test binary's name, and Windows uses `\r\n` line endings:

```text
Usage: greetcli.test [OPTION]... Files

Options:
  -verbose=[bool]
    -v
  -format=[string] (default: "text")
  -since=[greetcli_test.Date]: YYYY-MM-DD
  -tag=[[]string] (seperator: ,)

Arguments:
  Files [[]string]


Invalid input:
	flag: format

[[enum-error] invalid enumeration value] format got the value of xml which is not part of the acceptable values

accepted values:
 - text
 - json
```
