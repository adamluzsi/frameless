package cli_test

import (
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

const mwNameEnvKey = "CLI_TEST_MW_NAME" // the env tag of MWName.Name
const mwUserEnvKey = "CLI_TEST_MW_USER" // the env tag of MWConnect.User

type MWTrace struct {
	Calls          []string
	ConnectAtBuild MWConnect
	// Conn is the connection the command saw when it was executed.
	Conn *MWConn
}

// MWName is a Middleware with a value receiver and a configurable input.
type MWName struct {
	Trace *MWTrace `opaque:"true"`
	Name  string   `env:"CLI_TEST_MW_NAME" default:"mw-default"`
}

func (c MWName) MiddlewareCLI(next cli.Handler) cli.Handler {
	return cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
		c.Trace.Calls = append(c.Trace.Calls, c.Name)
		next.ServeCLI(w, r)
	})
}

// MWNested is a Middleware with a pointer receiver, which holds a Middleware field as well.
type MWNested struct {
	Trace   *MWTrace `opaque:"true"`
	Inner   MWName
	Connect MWConnect
	conn    *MWConn
}

func (c *MWNested) MiddlewareCLI(next cli.Handler) cli.Handler {
	c.Trace.ConnectAtBuild = c.Connect
	return cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
		c.Trace.Calls = append(c.Trace.Calls, "nested")
		c.conn = c.Connect.conn
		next.ServeCLI(w, r)
	})
}

type MWConn struct{ Addr string }

// MWConnect is a Middleware with a pointer receiver that sets up a dependency on its own unexported field,
// like a client made from the configuration, for the next handlers to use.
type MWConnect struct {
	Addr     string `flag:"addr"`
	User     string `env:"CLI_TEST_MW_USER" optional:"true"`
	Database string `arg:"0" optional:"true"`
	Driver   string `flag:"driver" default:"postgres"`
	conn     *MWConn
}

func (c *MWConnect) MiddlewareCLI(next cli.Handler) cli.Handler {
	return cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
		c.conn = &MWConn{Addr: c.Addr}
		next.ServeCLI(w, r)
	})
}

// MWGuard is a Middleware that rejects the request with cli.Error when it has a Reason,
// instead of calling the next Handler.
type MWGuard struct{ Reason string }

func (c MWGuard) MiddlewareCLI(next cli.Handler) cli.Handler {
	return cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
		if c.Reason != "" {
			cli.Error(w, c.Reason, cli.ExitCodeError)
			return
		}
		next.ServeCLI(w, r)
	})
}

// MWPromoting is a Middleware through the MiddlewareCLI method it promotes from the Middleware it embeds.
type MWPromoting struct{ MWName }

// MWNil is a Middleware that returns a nil Handler when Nil is set, which is a programming error.
type MWNil struct{ Nil bool }

func (c MWNil) MiddlewareCLI(next cli.Handler) cli.Handler {
	if c.Nil {
		return nil
	}
	return next
}

type MWCommand struct {
	MWName
	Nested    MWNested
	Last      MWName
	Promoting MWPromoting
	// Shared and SharedToo hold the same Middleware.
	Shared    *MWName
	SharedToo *MWName
	Guard     MWGuard
	Broken    MWNil
	Trace     *MWTrace `opaque:"true"`
}

func (cmd MWCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Trace.Calls = append(cmd.Trace.Calls, "cmd")
	cmd.Trace.Conn = cmd.Nested.conn
}

type mwState struct {
	open   bool
	schema string
	used   bool
}

type mwSubConfig struct {
	state *mwState
}

func (c *mwSubConfig) MiddlewareCLI(next cli.Handler) cli.Handler {
	return cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
		conn := &mwState{open: true}
		c.state = conn
		defer func() {
			conn.open = false
			fmt.Fprintf(w, "close: schema=%s used=%t open=%t\n", conn.schema, conn.used, conn.open)
		}()
		next.ServeCLI(w, r)
	})
}

type mwAppConfig struct {
	Sub    mwSubConfig
	Schema string `flag:"schema"`
	label  string
}

func (c *mwAppConfig) MiddlewareCLI(next cli.Handler) cli.Handler {
	return cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
		if c.Sub.state == nil || !c.Sub.state.open {
			cli.Error(w, "app config needs an open connection", cli.ExitCodeError)
			return
		}
		c.Sub.state.schema = c.Schema
		c.label = "report/" + c.Schema
		next.ServeCLI(w, r)
	})
}

type mwReportCommand struct{ Config *mwAppConfig }

func (cmd mwReportCommand) ServeCLI(w cli.ResponseWriter, _ *cli.Request) {
	conn := cmd.Config.Sub.state
	fmt.Fprintf(w, "handle: label=%s schema=%s open=%t\n", cmd.Config.label, conn.schema, conn.open)
	conn.used = true
}

func TestMiddleware(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		trace = let.Var(s, func(t *testcase.T) *MWTrace {
			return &MWTrace{}
		})
		embeddedName  = let.StringNC(s, 8, random.CharsetAlpha())
		innerName     = let.StringNC(s, 8, random.CharsetAlpha())
		lastName      = let.StringNC(s, 8, random.CharsetAlpha())
		promotingName = let.StringNC(s, 8, random.CharsetAlpha())
		sharedName    = let.StringNC(s, 8, random.CharsetAlpha())
		connAddr      = let.StringNC(s, 8, random.CharsetAlpha())
		guardReason   = let.VarOf(s, "")
		nilHandler    = let.VarOf(s, false)
	)
	handler := let.Var(s, func(t *testcase.T) MWCommand {
		shared := &MWName{Trace: trace.Get(t), Name: sharedName.Get(t)}
		return MWCommand{
			MWName: MWName{Trace: trace.Get(t), Name: embeddedName.Get(t)},
			Nested: MWNested{
				Trace:   trace.Get(t),
				Inner:   MWName{Trace: trace.Get(t), Name: innerName.Get(t)},
				Connect: MWConnect{Addr: connAddr.Get(t)},
			},
			Last:      MWName{Trace: trace.Get(t), Name: lastName.Get(t)},
			Promoting: MWPromoting{MWName: MWName{Trace: trace.Get(t), Name: promotingName.Get(t)}},
			Shared:    shared,
			SharedToo: shared,
			Guard:     MWGuard{Reason: guardReason.Get(t)},
			Broken:    MWNil{Nil: nilHandler.Get(t)},
			Trace:     trace.Get(t),
		}
	})
	count := func(t *testcase.T, call string) int {
		return len(slicekit.Filter(trace.Get(t).Calls, func(c string) bool { return c == call }))
	}

	givenConnectInputs := func(t *testcase.T, req *cli.Request) MWConnect {
		want := MWConnect{
			Addr:     "db-" + t.Random.StringNC(8, random.CharsetAlpha()),
			User:     t.Random.StringNC(8, random.CharsetAlpha()),
			Database: t.Random.StringNC(8, random.CharsetAlpha()),
			Driver:   "postgres",
		}
		testcase.SetEnv(t, mwUserEnvKey, want.User)
		req.Args = []string{"-addr", want.Addr, want.Database}
		return want
	}

	s.Before(func(t *testcase.T) {
		testcase.UnsetEnv(t, mwNameEnvKey)
		testcase.UnsetEnv(t, mwUserEnvKey)
		t.OnFail(func() { t.Log("calls:", trace.Get(t).Calls) })
	})

	s.Describe("ServeCLI", func(s *testcase.Spec) {
		var (
			command  = let.Var(s, func(t *testcase.T) cli.Handler { return handler.Get(t) })
			response = let.Var(s, func(t *testcase.T) *cli.ResponseRecorder {
				return &cli.ResponseRecorder{}
			})
			request = let.Var(s, func(t *testcase.T) *cli.Request {
				return &cli.Request{}
			})
		)
		act := func(t *testcase.T) {
			cli.ServeCLI(command.Get(t), response.Get(t), request.Get(t))
		}

		s.Before(func(t *testcase.T) {
			t.OnFail(func() {
				t.Log("code:", response.Get(t).Code)
				t.Log("\nout:\n", response.Get(t).Out.String())
				t.Log("\nerr:\n", response.Get(t).Err.String())
			})
		})

		s.When("a value command depends on an app config with a nested sub config middleware", func(s *testcase.Spec) {
			command.Let(s, func(t *testcase.T) cli.Handler { return mwReportCommand{Config: &mwAppConfig{}} })
			request.Let(s, func(t *testcase.T) *cli.Request { return &cli.Request{Args: []string{"-schema=analytics"}} })

			s.Then("the handler uses the configured connection and sub config teardown sees the same session's mutations", func(t *testcase.T) {
				act(t)

				rr := response.Get(t)
				assert.Equal(t, rr.Code, cli.ExitCodeOK, assert.Message(rr.Err.String()))
				assert.Equal(t, rr.Out.String(),
					"handle: label=report/analytics schema=analytics open=true\n"+
						"close: schema=analytics used=true open=false\n")
			})
		})

		s.Then("the deepest middleware fields run first, preserving declaration order among siblings", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.Equal(t, trace.Get(t).Calls, []string{
				innerName.Get(t), embeddedName.Get(t), "nested", lastName.Get(t), promotingName.Get(t), sharedName.Get(t), "cmd",
			})
		})

		s.Then("the parent MiddlewareCLI sees the nested flag, env, positional arg and default values before returning a handler", func(t *testcase.T) {
			want := givenConnectInputs(t, request.Get(t))
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.Equal(t, trace.Get(t).ConnectAtBuild, want)
		})

		s.Then("a middleware can use its initialized dependency before calling the value-receiver command", func(t *testcase.T) {
			act(t)

			assert.NotNil(t, trace.Get(t).Conn)
			assert.Equal(t, trace.Get(t).Conn.Addr, connAddr.Get(t))
		})

		s.Then("a middleware embedded into a middleware field is executed once, through the field that embeds it", func(t *testcase.T) {
			act(t)

			assert.Equal(t, count(t, promotingName.Get(t)), 1)
		})

		s.Then("a middleware held by more than one field is executed once", func(t *testcase.T) {
			act(t)

			assert.Equal(t, count(t, sharedName.Get(t)), 1)
		})

		s.When("a field typed any holds a middleware", func(s *testcase.Spec) {
			command.Let(s, func(t *testcase.T) cli.Handler {
				type cmd struct {
					cli.HandlerFunc
					Field any
				}
				return cmd{
					HandlerFunc: handler.Get(t).ServeCLI,
					Field:       &MWName{Trace: trace.Get(t), Name: "dynamic"},
				}
			})

			s.Then("the dynamic middleware runs before the command", func(t *testcase.T) {
				assert.NotPanic(t, func() { act(t) })

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.Equal(t, trace.Get(t).Calls, []string{"dynamic", "cmd"})
			})
		})

		s.When("a shallow field and a deeper middleware's child point to the same middleware", func(s *testcase.Spec) {
			command.Let(s, func(t *testcase.T) cli.Handler {
				type parent struct {
					MWName
					Child *MWName
				}
				type cmd struct {
					cli.HandlerFunc
					Shallow *MWName
					Group   struct{ Parent parent }
				}
				child := &MWName{Trace: trace.Get(t), Name: "child"}
				h := cmd{HandlerFunc: handler.Get(t).ServeCLI, Shallow: child}
				h.Group.Parent = parent{MWName: MWName{Trace: trace.Get(t), Name: "parent"}, Child: child}
				return h
			})

			s.Then("the shared child runs exactly once, before its parent and then the command", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.Equal(t, trace.Get(t).Calls, []string{"child", "parent", "cmd"})
			})
		})

		s.When("the middleware inputs are not set", func(s *testcase.Spec) {
			embeddedName.LetValue(s, "")
			innerName.LetValue(s, "")
			lastName.LetValue(s, "")
			promotingName.LetValue(s, "")
			sharedName.LetValue(s, "")

			s.Then("the middlewares are built with the default tag values", func(t *testcase.T) {
				act(t)

				assert.Equal(t, trace.Get(t).Calls, []string{
					"mw-default", "mw-default", "nested", "mw-default", "mw-default", "mw-default", "cmd",
				})
			})

			s.And("the middleware inputs are provided through the environment", func(s *testcase.Spec) {
				envName := let.StringNC(s, 8, random.CharsetAlpha())

				s.Before(func(t *testcase.T) {
					testcase.SetEnv(t, mwNameEnvKey, envName.Get(t))
				})

				s.Then("the middlewares are built with the environment values", func(t *testcase.T) {
					act(t)

					assert.Equal(t, trace.Get(t).Calls, []string{
						envName.Get(t), envName.Get(t), "nested", envName.Get(t), envName.Get(t), envName.Get(t), "cmd",
					})
				})
			})
		})

		s.When("a middleware rejects the request", func(s *testcase.Spec) {
			guardReason.Let(s, func(t *testcase.T) string {
				return t.Random.StringNC(8, random.CharsetAlpha())
			})

			s.Then("the command is not executed", func(t *testcase.T) {
				act(t)

				assert.NotContains(t, trace.Get(t).Calls, "cmd")
			})

			s.Then("the response is the error reply of the middleware", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeError)
				assert.Contains(t, response.Get(t).Err.String(), guardReason.Get(t))
			})
		})

		s.When("a middleware returns a nil handler", func(s *testcase.Spec) {
			nilHandler.LetValue(s, true)

			s.Then("it panics, as it is a programming error", func(t *testcase.T) {
				assert.Panic(t, func() { act(t) })

				assert.NotContains(t, trace.Get(t).Calls, "cmd")
			})
		})
	})

	s.Describe("Build", func(s *testcase.Spec) {
		var (
			ptr = let.Var(s, func(t *testcase.T) *MWCommand {
				h := handler.Get(t)
				return &h
			})
			request = let.Var(s, func(t *testcase.T) *cli.Request {
				return &cli.Request{}
			})
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		s.Then("the returned handler runs the deepest middleware fields first, preserving declaration order among siblings", func(t *testcase.T) {
			h, err := act(t)
			assert.NoError(t, err)

			h.ServeCLI(&cli.ResponseRecorder{}, &cli.Request{})
			assert.Equal(t, trace.Get(t).Calls, []string{
				innerName.Get(t), embeddedName.Get(t), "nested", lastName.Get(t), promotingName.Get(t), sharedName.Get(t), "cmd",
			})
		})

		s.Then("the parent MiddlewareCLI sees the nested flag, env, positional arg and default values before returning a handler", func(t *testcase.T) {
			want := givenConnectInputs(t, request.Get(t))
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, trace.Get(t).ConnectAtBuild, want)
			assert.Empty(t, trace.Get(t).Calls)
		})

		s.Then("a middleware of the returned handler can use its initialized dependency before calling the value-receiver command", func(t *testcase.T) {
			h, err := act(t)
			assert.NoError(t, err)

			h.ServeCLI(&cli.ResponseRecorder{}, &cli.Request{})
			assert.NotNil(t, trace.Get(t).Conn)
			assert.Equal(t, trace.Get(t).Conn.Addr, connAddr.Get(t))
		})

		s.Then("a middleware embedded into a middleware field of the returned handler is executed once, through the field that embeds it", func(t *testcase.T) {
			h, err := act(t)
			assert.NoError(t, err)

			h.ServeCLI(&cli.ResponseRecorder{}, &cli.Request{})
			assert.Equal(t, count(t, promotingName.Get(t)), 1)
		})

		s.Then("a middleware held by more than one field of the returned handler is executed once", func(t *testcase.T) {
			h, err := act(t)
			assert.NoError(t, err)

			h.ServeCLI(&cli.ResponseRecorder{}, &cli.Request{})
			assert.Equal(t, count(t, sharedName.Get(t)), 1)
		})

		s.Then("neither the middlewares nor the command are executed during the build", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Empty(t, trace.Get(t).Calls)
			assert.Nil(t, ptr.Get(t).Nested.Connect.conn)
		})

		s.When("the middleware inputs are not set", func(s *testcase.Spec) {
			embeddedName.LetValue(s, "")
			innerName.LetValue(s, "")
			lastName.LetValue(s, "")
			promotingName.LetValue(s, "")
			sharedName.LetValue(s, "")

			s.Then("the handler behind the pointer is configured with the default tag values", func(t *testcase.T) {
				_, err := act(t)
				assert.NoError(t, err)

				assert.Equal(t, ptr.Get(t).MWName.Name, "mw-default")
				assert.Equal(t, ptr.Get(t).Nested.Inner.Name, "mw-default")
				assert.Equal(t, ptr.Get(t).Last.Name, "mw-default")
				assert.Equal(t, ptr.Get(t).Promoting.Name, "mw-default")
				assert.Equal(t, ptr.Get(t).Shared.Name, "mw-default")
			})

			s.Then("the middlewares are built with the default tag values", func(t *testcase.T) {
				h, err := act(t)
				assert.NoError(t, err)

				h.ServeCLI(&cli.ResponseRecorder{}, &cli.Request{})
				assert.Equal(t, trace.Get(t).Calls, []string{
					"mw-default", "mw-default", "nested", "mw-default", "mw-default", "mw-default", "cmd",
				})
			})

			s.And("the middleware inputs are provided through the environment", func(s *testcase.Spec) {
				envName := let.StringNC(s, 8, random.CharsetAlpha())

				s.Before(func(t *testcase.T) {
					testcase.SetEnv(t, mwNameEnvKey, envName.Get(t))
				})

				s.Then("the middlewares are built with the environment values", func(t *testcase.T) {
					h, err := act(t)
					assert.NoError(t, err)

					h.ServeCLI(&cli.ResponseRecorder{}, &cli.Request{})
					assert.Equal(t, trace.Get(t).Calls, []string{
						envName.Get(t), envName.Get(t), "nested", envName.Get(t), envName.Get(t), envName.Get(t), "cmd",
					})
				})
			})
		})

		s.When("a middleware rejects the request", func(s *testcase.Spec) {
			guardReason.Let(s, func(t *testcase.T) string {
				return t.Random.StringNC(8, random.CharsetAlpha())
			})

			s.Then("the returned handler replies with the error of the middleware instead of executing the command", func(t *testcase.T) {
				h, err := act(t)
				assert.NoError(t, err)

				rr := &cli.ResponseRecorder{}
				h.ServeCLI(rr, &cli.Request{})
				assert.NotContains(t, trace.Get(t).Calls, "cmd")
				assert.Equal(t, rr.Code, cli.ExitCodeError)
				assert.Contains(t, rr.Err.String(), guardReason.Get(t))
			})
		})

		s.When("a middleware returns a nil handler", func(s *testcase.Spec) {
			nilHandler.LetValue(s, true)

			s.Then("it panics, as it is a programming error", func(t *testcase.T) {
				assert.Panic(t, func() { _, _ = act(t) })
			})
		})
	})
}
