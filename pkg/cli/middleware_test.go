package cli_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

const mwNameEnvKey = "CLI_TEST_MW_NAME" // the env tag of MWName.Name

type MWTrace struct {
	Calls []string
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
	Trace *MWTrace `opaque:"true"`
	Inner MWName
}

func (c *MWNested) MiddlewareCLI(next cli.Handler) cli.Handler {
	return cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
		c.Trace.Calls = append(c.Trace.Calls, "nested")
		next.ServeCLI(w, r)
	})
}

type MWConn struct{ Addr string }

// MWConnect is a Middleware with a pointer receiver that sets up a dependency on its own unexported field,
// like a client made from the configuration, for the next handlers to use.
type MWConnect struct {
	Addr string
	conn *MWConn
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
	MWConnect
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
	cmd.Trace.Conn = cmd.conn
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
			MWName:    MWName{Trace: trace.Get(t), Name: embeddedName.Get(t)},
			MWConnect: MWConnect{Addr: connAddr.Get(t)},
			Nested:    MWNested{Trace: trace.Get(t), Inner: MWName{Trace: trace.Get(t), Name: innerName.Get(t)}},
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

	s.Before(func(t *testcase.T) {
		testcase.UnsetEnv(t, mwNameEnvKey)
		t.OnFail(func() { t.Log("calls:", trace.Get(t).Calls) })
	})

	s.Describe("ServeCLI", func(s *testcase.Spec) {
		var (
			response = let.Var(s, func(t *testcase.T) *cli.ResponseRecorder {
				return &cli.ResponseRecorder{}
			})
			request = let.Var(s, func(t *testcase.T) *cli.Request {
				return &cli.Request{}
			})
		)
		act := func(t *testcase.T) {
			cli.ServeCLI(handler.Get(t), response.Get(t), request.Get(t))
		}

		s.Before(func(t *testcase.T) {
			t.OnFail(func() {
				t.Log("code:", response.Get(t).Code)
				t.Log("\nout:\n", response.Get(t).Out.String())
				t.Log("\nerr:\n", response.Get(t).Err.String())
			})
		})

		s.Then("the middleware fields wrap the command in field declaration order, outer before inner", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.Equal(t, trace.Get(t).Calls, []string{
				embeddedName.Get(t), "nested", innerName.Get(t), lastName.Get(t), promotingName.Get(t), sharedName.Get(t), "cmd",
			})
		})

		s.Then("a pointer receiver middleware can set up its field for the command, even when the command has a value receiver", func(t *testcase.T) {
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

		s.When("the middleware inputs are not set", func(s *testcase.Spec) {
			embeddedName.LetValue(s, "")
			innerName.LetValue(s, "")
			lastName.LetValue(s, "")
			promotingName.LetValue(s, "")
			sharedName.LetValue(s, "")

			s.Then("the middlewares are built with the default tag values", func(t *testcase.T) {
				act(t)

				assert.Equal(t, trace.Get(t).Calls, []string{
					"mw-default", "nested", "mw-default", "mw-default", "mw-default", "mw-default", "cmd",
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
						envName.Get(t), "nested", envName.Get(t), envName.Get(t), envName.Get(t), envName.Get(t), "cmd",
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

		s.Then("the returned handler is wrapped by the middleware fields in field declaration order, outer before inner", func(t *testcase.T) {
			h, err := act(t)
			assert.NoError(t, err)

			h.ServeCLI(&cli.ResponseRecorder{}, &cli.Request{})
			assert.Equal(t, trace.Get(t).Calls, []string{
				embeddedName.Get(t), "nested", innerName.Get(t), lastName.Get(t), promotingName.Get(t), sharedName.Get(t), "cmd",
			})
		})

		s.Then("a pointer receiver middleware of the returned handler can set up its field for the command, even when the command has a value receiver", func(t *testcase.T) {
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
			assert.Nil(t, ptr.Get(t).conn)
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
					"mw-default", "nested", "mw-default", "mw-default", "mw-default", "mw-default", "cmd",
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
						envName.Get(t), "nested", envName.Get(t), envName.Get(t), envName.Get(t), envName.Get(t), "cmd",
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
