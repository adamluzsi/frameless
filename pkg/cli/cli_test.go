package cli_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func Example() {
	var mux cli.Mux

	mux.Handle("foo", FooCommand{})

	sub := mux.Sub("sub")
	sub.Handle("subcmd", SubCommand{})

	cli.Main(context.Background(), mux)
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

	Arg    string `arg:"0" desc:"something something"`
	OthArg int    `arg:"1" default:"42"`

	// Dependency is a dependency of the FooCommand, which is populated though traditional dependency injection.
	Dependency string
}

func (cmd FooCommand) ServeCLI(w cli.Response, r *cli.Request) {
	fmt.Fprintln(w, "hello")
}

type SubCommand struct{}

func (cmd SubCommand) ServeCLI(w cli.Response, r *cli.Request) {
	w.Write([]byte("sub-cmd"))
}

///////////////////////////////////////////////////////////////////////////////////////////////////

var CommandNameCharset = random.CharsetAlpha() + random.CharsetDigit()

func TestMux(t *testing.T) {
	s := testcase.NewSpec(t)

	mux := testcase.Let(s, func(t *testcase.T) *cli.Mux {
		return &cli.Mux{}
	})

	s.Describe("#ServeCLI", func(s *testcase.Spec) {
		var (
			response = testcase.Let(s, func(t *testcase.T) *cli.ResponseRecorder {
				return &cli.ResponseRecorder{}
			})
			request = testcase.Let[*cli.Request](s, func(t *testcase.T) *cli.Request {
				return &cli.Request{}
			})
		)
		act := func(t *testcase.T) {
			mux.Get(t).ServeCLI(response.Get(t), request.Get(t))
		}

		s.Before(func(t *testcase.T) {
			args := slicekit.Clone(request.Get(t).Args)
			t.OnFail(func() {
				t.Log("args:", args)
				t.Log("code:", response.Get(t).Code)
				t.Log("\nout:\n", response.Get(t).Out.String())
				t.Log("\nerr:\n", response.Get(t).Err.String())
			})
		})

		s.Describe("routing", func(s *testcase.Spec) {
			var ( // given we have a command
				commandName = let.StringNC(s, 5, CommandNameCharset)
				ExpCmdReply = let.String(s)
				command     = testcase.Let(s, func(t *testcase.T) cli.Handler {
					return StubServeCLIFunc(func(w cli.Response, r *cli.Request) {
						fmt.Fprint(w, ExpCmdReply.Get(t))
					})
				})
			)

			mux.Let(s, func(t *testcase.T) *cli.Mux {
				m := mux.Super(t)
				m.Handle(commandName.Get(t), command.Get(t))
				return m
			})

			request.Let(s, func(t *testcase.T) *cli.Request {
				t.Log("given the args start with the command's name")
				r := request.Super(t)
				r.Args = append([]string{commandName.Get(t)}, r.Args...)
				return r
			})

			s.Then("then the command is executed", func(t *testcase.T) {
				var (
					lastRequest   *cli.Request
					expectedOuput = t.Random.String()
				)
				command.Set(t, StubServeCLIFunc(func(w cli.Response, r *cli.Request) {
					lastRequest = r
					w.ExitCode(42)
					w.Write([]byte(expectedOuput))
				}))

				act(t)

				assert.NotNil(t, lastRequest)
				assert.Equal(t, response.Get(t).Out.String(), expectedOuput)
			})

			s.Test("E2E", func(t *testcase.T) {
				// flags
				request.Get(t).Args = append(request.Get(t).Args,
					/* flags */ "-str", "foo", "-int", "42", "-bool=true", "-sbool", "-fbool=0",
					/* args */ "hello-world", "42", "True",
				)

				t.OnFail(func() { t.Log("args:", request.Get(t).Args) })

				var cmd CommandE2E
				command.Set(t, CommandE2E{Callback: func(h CommandE2E, w cli.Response, r *cli.Request) {
					cmd = h
					fmt.Fprintln(w, "out")
					fmt.Fprintln(w.(cli.ErrorWriter).Stderr(), "errout")
				}})

				act(t)

				assert.Contain(t, response.Get(t).Out.String(), "out")
				assert.Contain(t, response.Get(t).Err.String(), "errout")

				assert.Equal(t, cmd.Flag1, "foo")
				assert.Equal(t, cmd.Flag2, "defval")
				assert.Equal(t, cmd.Flag3, 42)
				assert.Equal(t, cmd.Flag4, true)
				assert.Equal(t, cmd.Flag5, true)
				assert.Equal(t, cmd.Flag6, false)

				assert.Equal(t, cmd.Arg1, "hello-world")
				assert.Equal(t, cmd.Arg2, 42)
				assert.Equal(t, cmd.Arg3, true)
			})

			s.Test("support for pointer types", func(t *testcase.T) {
				// flags
				request.Get(t).Args = append(request.Get(t).Args,
					/* flags */ "-flag", "foo",
					/* args */ "hello-world",
				)

				command.Set(t, &CommandWithPointerReceiver{Callback: func(h *CommandWithPointerReceiver, w cli.Response, r *cli.Request) {
					assert.NotNil(t, h)
					assert.NotEmpty(t, h.Flag)
					assert.NotEmpty(t, h.Arg)
					fmt.Fprint(w, "teapot")
				}})

				act(t)

				assert.Contain(t, response.Get(t).Out.String(), "teapot")
			})

			s.And("the command have flags", func(s *testcase.Spec) {
				var h = testcase.LetValue[*CommandWithFlag](s, nil)

				command.Let(s, func(t *testcase.T) cli.Handler {
					return CommandWithFlag{Callback: func(v CommandWithFlag, w cli.Response, r *cli.Request) {
						h.Set(t, &v)

						fmt.Fprintln(w, ExpCmdReply.Get(t))
					}}
				})

				s.And("one of the flag is a bool type", func(s *testcase.Spec) {
					var _ bool = CommandWithFlag{}.Flag5

					s.And("the flag value is provided as a boolean value", func(s *testcase.Spec) {
						exp := let.Bool(s)

						request.Let(s, func(t *testcase.T) *cli.Request {
							var v string
							if exp.Get(t) {
								v = random.Pick(t.Random, "1", "t", "T", "true", "TRUE", "True")
							} else {
								v = random.Pick(t.Random, "0", "f", "F", "false", "FALSE", "False")
							}
							var arg string = "-bool=" + v
							t.OnFail(func() { t.Log("arg:", arg) })
							r := request.Super(t)
							r.Args = append(r.Args, arg)
							return r
						})

						s.Then("we expect that the boolean value is parsed successfully", func(t *testcase.T) {
							act(t)

							assert.NotNil(t, h.Get(t))
							assert.Equal(t, h.Get(t).Flag5, exp.Get(t))
						})
					})

					s.And("the flag value is not supplied only the flag itself", func(s *testcase.Spec) {
						request.Let(s, func(t *testcase.T) *cli.Request {
							r := request.Super(t)
							r.Args = append(r.Args, "-bool", "-noise", "wow")
							return r
						})

						s.Then("the bool flag is interpreted as enabled, so by cli convention it will default to True", func(t *testcase.T) {
							act(t)

							assert.NotNil(t, h.Get(t))
							assert.Empty(t, response.Get(t).Err.String())
							assert.True(t, h.Get(t).Flag5)
						})
					})
				})

				AndTheFlagTypeIs[string]{
					Act:      act,
					Response: response,
					Request:  request,
					Flag:     "-str",
					Expected: func(t *testcase.T) string {
						return t.Random.String()
					},
					Got: func(t *testcase.T) string {
						return h.Get(t).FlagStr
					},
				}.Spec(s)

				AndTheFlagTypeIs[SubStr]{
					Act:      act,
					Desc:     "where string is a subtype",
					Response: response,
					Request:  request,
					Flag:     "-substr",
					Expected: func(t *testcase.T) SubStr {
						return SubStr(t.Random.String())
					},
					Got: func(t *testcase.T) SubStr {
						return h.Get(t).FlagSubStr
					},
				}.Spec(s)

				AndTheFlagTypeIs[string]{
					Desc:     "with default value",
					Act:      act,
					Response: response,
					Request:  request,
					Flag:     "-strwd",
					Default:  "configured-default-value",
					Expected: func(t *testcase.T) string {
						return string(t.Random.String())
					},
					Got: func(t *testcase.T) string {
						return h.Get(t).FlagStrWD
					},
				}.Spec(s)

				AndTheFlagTypeIs[int]{
					Act:      act,
					Response: response,
					Request:  request,
					Flag:     "-int",
					Expected: func(t *testcase.T) int {
						return t.Random.Int()
					},
					Got: func(t *testcase.T) int {
						return h.Get(t).FlagInt
					},
				}.Spec(s)

				AndTheFlagTypeIs[int]{
					Act:      act,
					Response: response,
					Request:  request,
					Flag:     "-intwd",
					Default:  42,
					Expected: func(t *testcase.T) int {
						return t.Random.Int()
					},
					Got: func(t *testcase.T) int {
						return h.Get(t).FlagIntWD
					},
				}.Spec(s)

				s.And("the flag is required type", func(s *testcase.Spec) {
					h := testcase.LetValue[*CommandWithFlagWithRequired[string]](s, nil)

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithFlagWithRequired[string]{
							Callback: func(v CommandWithFlagWithRequired[string], w cli.Response, r *cli.Request) {
								h.Set(t, &v)
							},
						}
					})

					AndTheFlagTypeIs[string]{
						IsRequired: true,

						Act:      act,
						Response: response,
						Request:  request,
						Flag:     "-flag",
						Expected: func(t *testcase.T) string {
							return t.Random.String()
						},
						Got: func(t *testcase.T) string {
							return h.Get(t).Flag
						},
					}.Spec(s)
				})

				s.And("the flag has an enum limitation", func(s *testcase.Spec) {
					h := testcase.LetValue[*CommandWithFlagWithEnum](s, nil)

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithFlagWithEnum{
							Callback: func(v CommandWithFlagWithEnum, w cli.Response, r *cli.Request) {
								h.Set(t, &v)
							},
						}
					})

					s.And("the input value is within the enum set", func(s *testcase.Spec) {
						exp := let.ElementFrom(s, "foo", "bar", "baz")

						request.Let(s, func(t *testcase.T) *cli.Request {
							r := request.Super(t)
							r.Args = append(r.Args, "-flag", exp.Get(t))
							return r
						})

						s.Then("it will be accepted", func(t *testcase.T) {
							act(t)

							assert.Equal(t, cli.ExitCodeOK, response.Get(t).Code)
							assert.Empty(t, response.Get(t).Err.String())

							assert.Equal(t, h.Get(t).Flag, exp.Get(t))
						})
					})

					s.And("the input value is not within the enum set", func(s *testcase.Spec) {
						request.Let(s, func(t *testcase.T) *cli.Request {
							r := request.Super(t)
							r.Args = append(r.Args, "-flag", random.Pick(t.Random, "qux", "quux"))
							return r
						})

						s.Then("the exit code will indenticate a bad usage", func(t *testcase.T) {
							act(t)

							assert.NotEmpty(t, response.Get(t).Err.String())
						})

						s.Then("the error ouput will be written", func(t *testcase.T) {
							act(t)

							errOutput := response.Get(t).Err.String()
							assert.NotEmpty(t, errOutput)
							assert.Contain(t, errOutput, cli.ErrFlagInvalid)
						})

						s.Then("the the error response will mention valid enum values", func(t *testcase.T) {
							act(t)

							field, ok := reflectkit.TypeOf[CommandWithFlagWithEnum]().FieldByName("Flag")
							assert.True(t, ok, "expected to find the struct value")
							vs, err := enum.ReflectValuesOfStructField(field)
							assert.NoError(t, err)
							assert.NotEmpty(t, vs)

							var _ string = CommandWithFlagWithEnum{}.Flag // static check for Flag field type
							for _, v := range vs {
								assert.Contain(t, response.Get(t).Err.String(), v.String())
							}
						})
					})
				})
			})

			s.When("the command defines an argument(s)", func(s *testcase.Spec) {
				s.Context("string", func(s *testcase.Spec) {
					h := testcase.LetValue[*CommandWithArg[string]](s, nil)

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithArg[string]{Callback: func(v CommandWithArg[string], w cli.Response, r *cli.Request) { h.Set(t, &v) }}
					})

					AndTheArgTypeIs[string]{
						Act:      act,
						Response: response,
						Request:  request,
						Expected: func(t *testcase.T) string {
							return t.Random.String()
						},
						Got: func(t *testcase.T) string {
							return h.Get(t).Arg
						},
					}.Spec(s)
				})

				s.Context("int", func(s *testcase.Spec) {
					h := testcase.LetValue[*CommandWithArg[int]](s, nil)

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithArg[int]{Callback: func(v CommandWithArg[int], w cli.Response, r *cli.Request) { h.Set(t, &v) }}
					})

					AndTheArgTypeIs[int]{
						Act:      act,
						Response: response,
						Request:  request,
						Expected: func(t *testcase.T) int {
							return t.Random.Int()
						},
						Got: func(t *testcase.T) int {
							return h.Get(t).Arg
						},
					}.Spec(s)
				})

				s.Context("bool", func(s *testcase.Spec) {
					h := testcase.LetValue[*CommandWithArg[bool]](s, nil)

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithArg[bool]{Callback: func(v CommandWithArg[bool], w cli.Response, r *cli.Request) { h.Set(t, &v) }}
					})

					AndTheArgTypeIs[bool]{
						Act:      act,
						Response: response,
						Request:  request,
						Expected: func(t *testcase.T) bool {
							return t.Random.Bool()
						},
						Got: func(t *testcase.T) bool {
							return h.Get(t).Arg
						},
					}.Spec(s)
				})

				s.Context("along with a default", func(s *testcase.Spec) {
					h := testcase.LetValue[*CommandWithArgWithDefault](s, nil)

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithArgWithDefault{Callback: func(v CommandWithArgWithDefault, w cli.Response, r *cli.Request) { h.Set(t, &v) }}
					})

					AndTheArgTypeIs[string]{
						Default: "defval",

						Act:      act,
						Response: response,
						Request:  request,
						Expected: func(t *testcase.T) string { return t.Random.String() },
						Got:      func(t *testcase.T) string { return h.Get(t).Arg },
					}.Spec(s)
				})

				s.Context("which is mandatory/required", func(s *testcase.Spec) {
					h := testcase.Let[*CommandWithArgWithRequired[string]](s, func(t *testcase.T) *CommandWithArgWithRequired[string] {
						return &CommandWithArgWithRequired[string]{}
					})

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithArgWithRequired[string]{Callback: func(v CommandWithArgWithRequired[string], w cli.Response, r *cli.Request) { h.Set(t, &v) }}
					})

					AndTheArgTypeIs[string]{
						IsRequired: true,

						Act:      act,
						Response: response,
						Request:  request,
						Expected: func(t *testcase.T) string { return t.Random.String() },
						Got:      func(t *testcase.T) string { return h.Get(t).Arg },
					}.Spec(s)
				})

				s.And("the it has an enum limitation", func(s *testcase.Spec) {
					h := testcase.LetValue[*CommandWithArgWithEnum](s, nil)

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithArgWithEnum{
							Callback: func(v CommandWithArgWithEnum, w cli.Response, r *cli.Request) { h.Set(t, &v) },
						}
					})

					s.And("the input value is within the enum set", func(s *testcase.Spec) {
						exp := let.ElementFrom(s, "foo", "bar", "baz")

						request.Let(s, func(t *testcase.T) *cli.Request {
							r := request.Super(t)
							r.Args = append(r.Args, exp.Get(t))
							return r
						})

						s.Then("it will be accepted", func(t *testcase.T) {
							act(t)

							assert.Equal(t, cli.ExitCodeOK, response.Get(t).Code)
							assert.Empty(t, response.Get(t).Err.String())

							assert.Equal(t, h.Get(t).Arg, exp.Get(t))
						})
					})

					s.And("the input value is not within the enum set", func(s *testcase.Spec) {
						request.Let(s, func(t *testcase.T) *cli.Request {
							r := request.Super(t)
							r.Args = append(r.Args, random.Pick(t.Random, "qux", "quux"))
							return r
						})

						s.Then("the exit code will indenticate a bad usage", func(t *testcase.T) {
							act(t)

							assert.NotEmpty(t, response.Get(t).Err.String())
						})

						s.Then("the error ouput will be written", func(t *testcase.T) {
							act(t)

							errOutput := response.Get(t).Err.String()
							assert.NotEmpty(t, errOutput)
							assert.Contain(t, strings.ToLower(errOutput), "usage")
							assert.AnyOf(t, func(a *assert.A) {
								a.Case(func(t assert.It) { assert.Contain(t, errOutput, "qux") })
								a.Case(func(t assert.It) { assert.Contain(t, errOutput, "quux") })
							})
						})

						s.Then("the the error response will mention valid enum values", func(t *testcase.T) {
							act(t)

							field, ok := reflectkit.TypeOf[CommandWithArgWithEnum]().FieldByName("Arg")
							assert.True(t, ok, "expected to find the struct value")
							vs, err := enum.ReflectValuesOfStructField(field)
							assert.NoError(t, err)
							assert.NotEmpty(t, vs)

							var _ string = CommandWithArgWithEnum{}.Arg // static check for Flag field type
							for _, v := range vs {
								assert.Contain(t, response.Get(t).Err.String(), v.String())
							}
						})
					})
				})
			})

			s.When("the command name is not passed in the args", func(s *testcase.Spec) {
				s.Context("because the args left empty", func(s *testcase.Spec) {
					request.Let(s, func(t *testcase.T) *cli.Request {
						r := request.Super(t)
						r.Args = []string{}
						return r
					})

					s.Then("bad request error code returned", func(t *testcase.T) {
						act(t)

						assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					})

					s.Then("error message is returned", func(t *testcase.T) {
						act(t)

						assert.Empty(t, response.Get(t).Out.String())
						assert.NotEmpty(t, response.Get(t).Err.String())
					})
				})

				s.Context("because the args point to an unknown command", func(s *testcase.Spec) {
					unknownCommandName := let.StringNC(s, 5, CommandNameCharset)

					request.Let(s, func(t *testcase.T) *cli.Request {
						r := request.Super(t)
						r.Args = []string{unknownCommandName.Get(t)}
						return r
					})

					s.Then("bad request error code returned", func(t *testcase.T) {
						act(t)

						assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					})

					s.Then("error output returned mentioning the unknown command", func(t *testcase.T) {
						act(t)

						assert.Empty(t, response.Get(t).Out.String())
						assert.NotEmpty(t, response.Get(t).Err.String())

						assert.Contain(t, response.Get(t).Err.String(), unknownCommandName.Get(t),
							"expected that the unknown command name is mentioned")
					})

					s.Context("but when other commands are registered", func(s *testcase.Spec) {
						mux.Let(s, func(t *testcase.T) *cli.Mux {
							m := mux.Super(t)
							m.Handle("foo", StubServeCLIFunc(func(w cli.Response, r *cli.Request) {}))
							m.Handle("bar", StubServeCLIFunc(func(w cli.Response, r *cli.Request) {}))
							m.Handle("baz", StubServeCLIFunc(func(w cli.Response, r *cli.Request) {}))
							return m
						})

						s.Then("available commands are suggested", func(t *testcase.T) {
							act(t)

							assert.Empty(t, response.Get(t).Out.String())
							assert.NotEmpty(t, response.Get(t).Err.String())

							const expectedToFindCommands = "expected that the available commands are listed"
							t.Log(response.Get(t).Err.String())
							assert.Contain(t, response.Get(t).Err.String(), "foo", expectedToFindCommands)
							assert.Contain(t, response.Get(t).Err.String(), "bar", expectedToFindCommands)
							assert.Contain(t, response.Get(t).Err.String(), "baz", expectedToFindCommands)
						})
					})
				})
			})
		})

		s.Describe("help", func(s *testcase.Spec) {
			request.Let(s, func(t *testcase.T) *cli.Request {
				r := request.Super(t)
				helpFlag := random.Pick(t.Random, "-help", "-h")
				t.OnFail(func() { t.Log("help flag:", helpFlag) })
				r.Args = append(r.Args, helpFlag)
				return r
			})

			var thenExitCodeIsOK = func(s *testcase.Spec) {
				s.Then("the exit code will be OK as help is explicitly requested", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				})
			}

			var thenUsagePrintedOutToSTDOUT = func(s *testcase.Spec) {
				s.Then("the usage will be printed to the stdout", func(t *testcase.T) {
					act(t)

					out := response.Get(t).Out.String()
					assert.NotEmpty(t, out)
					assert.Contain(t, strings.ToLower(out), "usage")
				})
			}

			thenExitCodeIsOK(s)

			thenUsagePrintedOutToSTDOUT(s)

			s.Then("the help request is not interpreted as an unknown command", func(t *testcase.T) {
				act(t)

				assert.NotContain(t, strings.ToLower(response.Get(t).Out.String()), "unknown")
			})

			s.When("commands are registered", func(s *testcase.Spec) {
				return
				mux.Let(s, func(t *testcase.T) *cli.Mux {
					m := mux.Super(t)
					m.Handle("e2e", CommandE2E{})
					m.Handle("foo", FooCommand{})
					return m
				})

				thenExitCodeIsOK(s)

				thenUsagePrintedOutToSTDOUT(s)

				s.Then("commands are listed", func(t *testcase.T) {
					act(t)

					out := response.Get(t).Out.String()
					assert.NotEmpty(t, out)
					assert.Contain(t, strings.ToLower(out), "commands")
					assert.Contain(t, out, "foo")
					assert.Contain(t, out, "e2e: "+CommandE2E{}.Summary())
				})

				s.And("and command is specified before the help flag", func(s *testcase.Spec) {
					request.Let(s, func(t *testcase.T) *cli.Request {
						r := request.Super(t)
						slicekit.Unshift(&r.Args, "e2e")
						return r
					})

					thenExitCodeIsOK(s)

					thenUsagePrintedOutToSTDOUT(s)

					s.Then("the command specific documentation is printed out", func(t *testcase.T) {
						act(t)

						out := response.Get(t).Out.String()
						_ = out

						assert.Contain(t, out, "")
					})
				})
			})
		})
	})
}

type AndTheFlagTypeIs[T any] struct {
	Desc     string
	Act      func(t *testcase.T)
	Response testcase.Var[*cli.ResponseRecorder]
	Request  testcase.Var[*cli.Request]

	Flag    string
	Default T

	Expected testcase.VarInit[T]
	Got      func(t *testcase.T) T

	IsRequired bool
}

func (c AndTheFlagTypeIs[T]) Spec(s *testcase.Spec) {
	var name = reflectkit.SymbolicName(reflectkit.TypeOf[T]())
	var desc = c.Desc
	if 0 < len(desc) {
		desc = " " + desc
	}

	s.And("the flag type is a "+name+" type"+desc, func(s *testcase.Spec) {

		s.And("the flag value is provided as a "+name+" value", func(s *testcase.Spec) {
			exp := testcase.Let(s, c.Expected)

			raw := testcase.Let(s, func(t *testcase.T) string {
				raw, err := convkit.Format(exp.Get(t))
				assert.NoError(t, err)
				return raw
			})

			c.Request.Let(s, func(t *testcase.T) *cli.Request {
				var args = []string{c.Flag, raw.Get(t)}
				t.OnFail(func() { t.Log("args:", args) })

				r := c.Request.Super(t)
				r.Args = append(r.Args, args...)
				return r
			})

			s.Then("we expect the value is parsed successfully", func(t *testcase.T) {
				c.Act(t)

				assert.Equal(t, c.Got(t), exp.Get(t))
			})

			s.Then("we expect that the request args no longer contain the raw flag input", func(t *testcase.T) {
				c.Act(t)

				assert.NotContain(t, c.Request.Get(t).Args, c.Flag)
				assert.NotContain(t, c.Request.Get(t).Args, raw.Get(t))
			})
		})

		s.And("the flag value is not supplied only the flag itself", func(s *testcase.Spec) {
			c.Request.Let(s, func(t *testcase.T) *cli.Request {
				r := c.Request.Super(t)
				r.Args = append(r.Args, c.Flag)
				return r
			})
			s.Then("it will cause an error", func(t *testcase.T) {
				c.Act(t)

				assert.Equal(t, cli.ExitCodeBadRequest, c.Response.Get(t).Code)
			})
		})

		s.And("the flag is not provided", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				for _, arg := range c.Request.Get(t).Args {
					assert.NotContain(t, arg, c.Flag)
				}
			})

			if c.IsRequired {
				s.Then("it will raise an error that the flag is required but missing", func(t *testcase.T) {
					c.Act(t)

					assert.Equal(t, c.Response.Get(t).Code, cli.ExitCodeBadRequest)
					assert.NotEmpty(t, c.Response.Get(t).Err.String())
				})
			} else {
				if zerokit.IsZero(c.Default) {
					s.Then("the flag remains in a zero state", func(t *testcase.T) {
						c.Act(t)

						assert.Empty(t, c.Response.Get(t).Err.String())
						assert.Empty(t, c.Got(t))
					})
				} else {
					s.Then("the flag will be set to the default value", func(t *testcase.T) {
						c.Act(t)

						assert.Empty(t, c.Response.Get(t).Err.String())
						assert.Equal(t, c.Default, c.Got(t))
					})
				}
			}
		})
	})
}

type AndTheArgTypeIs[T any] struct {
	Act      func(t *testcase.T)
	Response testcase.Var[*cli.ResponseRecorder]
	Request  testcase.Var[*cli.Request]

	Default T

	Expected testcase.VarInit[T]
	Got      func(t *testcase.T) T

	IsRequired bool
}

func (c AndTheArgTypeIs[T]) Spec(s *testcase.Spec) {
	var name = reflectkit.SymbolicName(reflectkit.TypeOf[T]())

	s.And("the ARG value is provided as a "+name+" value", func(s *testcase.Spec) {
		exp := testcase.Let(s, c.Expected)

		raw := testcase.Let(s, func(t *testcase.T) string {
			raw, err := convkit.Format(exp.Get(t))
			assert.NoError(t, err)
			return raw
		})

		c.Request.Let(s, func(t *testcase.T) *cli.Request {
			raw, err := convkit.Format(exp.Get(t))
			assert.NoError(t, err)

			r := c.Request.Super(t)
			r.Args = append(r.Args, "--", raw)

			args := slicekit.Clone(r.Args)
			t.OnFail(func() { t.Log("args:", args) })

			return r
		}).EagerLoading(s) // I'm not sure why this needs to be eager loaded

		s.Then("we expect the value is parsed successfully", func(t *testcase.T) {
			c.Act(t)

			defer errorkit.RecoverWith(func(r any) { t.Fail() })
			assert.Equal(t, c.Got(t), exp.Get(t))
		})

		s.Then("we expect that the request args no longer contain the raw flag input", func(t *testcase.T) {
			c.Act(t)

			assert.NotContain(t, c.Request.Get(t).Args, raw.Get(t))
		})
	})

	s.And("the ARG is not provided", func(s *testcase.Spec) {
		if c.IsRequired {
			s.Then("it will raise an error that the ARG is required but missing", func(t *testcase.T) {
				c.Act(t)

				assert.Equal(t, c.Response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.NotEmpty(t, c.Response.Get(t).Err.String())
			})
		} else {
			if zerokit.IsZero(c.Default) {
				s.Then("the ARG remains in a zero state", func(t *testcase.T) {
					c.Act(t)

					assert.Empty(t, c.Response.Get(t).Err.String())
					assert.Empty(t, c.Got(t))
				})
			} else {
				s.Then("the ARG will be set to the default value", func(t *testcase.T) {
					c.Act(t)

					assert.Empty(t, c.Response.Get(t).Err.String())
					assert.Equal(t, c.Default, c.Got(t))
				})
			}
		}
	})
}

type StubServeCLIFunc func(w cli.Response, r *cli.Request)

func (cmd StubServeCLIFunc) ServeCLI(w cli.Response, r *cli.Request) {
	if cmd != nil {
		cmd(w, r)
	}
}

type ExampleCommandWithFlags struct {
	Flag1 string `flag:"the-a,a" default:"val"   desc:"this is flag A"`
	Flag2 bool   `flag:"the-b,b" default:"true"` // missing description
	Flag3 int    `flag:"c" required:"true"       desc:"this is flag C, not B"`

	Arg1 string `arg:"0" desc:"something something"`
	Arg2 int    `arg:"1" default:"42"`

	// Dependency is a dependency of the FooCommand, which is populated though traditional dependency injection.
	Dependency any
}

type Callback[T any] func(v T, w cli.Response, r *cli.Request)

func (fn Callback[T]) Call(v T, w cli.Response, r *cli.Request) {
	if fn != nil {
		fn(v, w, r)
	}
}

type CommandE2E struct {
	Callback[CommandE2E]

	Flag1 string `flag:"str" desc:"flag1 desc"`
	Flag2 string `flag:"strwd" default:"defval"`
	Flag3 int    `flag:"int" env:"FLAG3"`
	Flag4 bool   `flag:"bool"`
	Flag5 bool   `flag:"sbool"`
	Flag6 bool   `flag:"fbool"`

	Arg1 string `arg:"0"`
	Arg2 int    `arg:"1"`
	Arg3 bool   `arg:"2"`
}

func (cmd CommandE2E) Summary() string { return "E2E command summary" }

func (cmd CommandE2E) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithPointerReceiver struct {
	Callback[*CommandWithPointerReceiver]
	Flag string `flag:"flag"`
	Arg  string `arg:"0"`
}

func (cmd *CommandWithPointerReceiver) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithFlag struct {
	Callback[CommandWithFlag]
	Noise string `flag:"noise"`

	FlagStr    string `flag:"str"`
	FlagStrWD  string `flag:"strwd" default:"configured-default-value"`
	FlagSubStr SubStr `flag:"substr"`

	FlagInt   int `flag:"int"`
	FlagIntWD int `flag:"intwd" default:"42"`

	Flag3 int  `flag:"c"`
	Flag4 int  `flag:"d" default:"4242"`
	Flag5 bool `flag:"bool"`
}

type SubStr string

func (cmd CommandWithFlag) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithFlagWithRequired[T any] struct {
	Callback[CommandWithFlagWithRequired[T]]
	Flag T `flag:"flag" required:"true"`
}

func (cmd CommandWithFlagWithRequired[T]) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithFlagWithEnv[T any] struct {
	Callback[CommandWithFlagWithEnv[T]]
	Flag T `flag:"flag" env:"FLAGNAME"`
}

func (cmd CommandWithFlagWithEnv[T]) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithFlagWithEnum struct {
	Callback[CommandWithFlagWithEnum]
	Flag string `flag:"flag" enum:"foo,bar,baz,"`
}

func (cmd CommandWithFlagWithEnum) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithArg[T any] struct {
	Callback[CommandWithArg[T]]
	Arg T `arg:"0"`
}

func (cmd CommandWithArg[T]) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithArgWithDefault struct {
	Callback[CommandWithArgWithDefault]
	Arg string `arg:"0" default:"defval"`
}

func (cmd CommandWithArgWithDefault) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithArgWithRequired[T any] struct {
	Callback[CommandWithArgWithRequired[T]]
	Arg T `arg:"0" required:"1"`
}

func (cmd CommandWithArgWithRequired[T]) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithArgWithEnum struct {
	Callback[CommandWithArgWithEnum]
	Arg string `arg:"0" enum:"foo,bar,baz,"`
}

func (cmd CommandWithArgWithEnum) ServeCLI(w cli.Response, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

func TestConfigureHandler(t *testing.T) {
	req := &cli.Request{Args: []string{
		/* flags */ "-str", "foo", "-int", "42", "-bool=true", "-sbool", "-fbool=0",
		/* args */ "hello-world", "42", "True",
	}}

	cmd, err := cli.ConfigureHandler(CommandE2E{}, "", req)
	assert.NoError(t, err)
	assert.NotEmpty(t, cmd)

	assert.Equal(t, cmd.Flag1, "foo")
	assert.Equal(t, cmd.Flag2, "defval")
	assert.Equal(t, cmd.Flag3, 42)
	assert.Equal(t, cmd.Flag4, true)
	assert.Equal(t, cmd.Flag5, true)
	assert.Equal(t, cmd.Flag6, false)

	assert.Equal(t, cmd.Arg1, "hello-world")
	assert.Equal(t, cmd.Arg2, 42)
	assert.Equal(t, cmd.Arg3, true)
}

func TestUsage(t *testing.T) {
	t.Run("struct", func(t *testing.T) {
		usage, err := cli.Usage(CommandE2E{}, "thepath")
		assert.NoError(t, err)

		assert.Contain(t, usage, "Usage: thepath [OPTION]... [Arg1] [Arg2] [Arg3]")
		assert.Contain(t, usage, "-str=[string]: flag1 desc")
		assert.Contain(t, usage, "-strwd=[string] (default: defval)")
		assert.Contain(t, usage, "-int=[int]")
		assert.Contain(t, usage, "-bool=[bool]")
		assert.Contain(t, usage, "-sbool=[bool]")
		assert.Contain(t, usage, "-fbool=[bool]")
		assert.Contain(t, usage, "Arg1 [string]")
		assert.Contain(t, usage, "Arg2 [int]")
		assert.Contain(t, usage, "Arg3 [bool]")
		assert.Contain(t, usage, "-int=[int] (env: FLAG3)", "env variable is mentioned")
	})
	t.Run("when cli.Handler#Usage(path) is supported", func(t *testing.T) {
		usage, err := cli.Usage(CommandWithUsageSupport{}, "thepath")
		assert.NoError(t, err)

		assert.Contain(t, usage, "Custom Usage Message: thepath")
	})
}

type CommandWithUsageSupport struct{}

func (CommandWithUsageSupport) Usage(path string) (string, error) {
	return fmt.Sprintf("Custom Usage Message: %s", path), nil
}

func (CommandWithUsageSupport) ServeCLI(w cli.Response, r *cli.Request) {}

type CommandWithDependency struct {
	Flag1 string `flag:"flag1" default:"val" desc:"this is flag A"`
	Flag2 bool   `flag:"flag2" default:"true"`
	Flag3 int    `flag:"othflag" required:"true" desc:"this is flag C, not B"`

	Arg1 string `arg:"0" desc:"something something"`
	Arg2 int    `arg:"1" default:"42" desc:"something something else"`

	// Dependency is a dependency of the FooCommand, which is populated though traditional dependency injection.
	Dependency any
}

func (CommandWithDependency) ServeCLI(w cli.Response, r *cli.Request) {}

func Example_dependencyInjection() {
	cli.Main(context.Background(), CommandWithDependency{
		Dependency: "important dependency that I need as part of the ServeCLI call",
	})
}

func TestConfigureHandler_requiredFlag_defaultValueDependencyInjected(t *testing.T) {
	cmd := CommandWithFlagWithRequired[string]{Flag: "42"}
	cmd.Callback = func(v CommandWithFlagWithRequired[string], w cli.Response, r *cli.Request) { cmd = v }

	cmd, err := cli.ConfigureHandler(cmd, "path", &cli.Request{})
	assert.NoError(t, err, "expected no error, since default value is already provided")
	assert.Equal(t, cmd.Flag, "42")
}

func TestConfigureHandler_requiredArg_injextedDefaultValue(t *testing.T) {
	cmd := CommandWithArgWithRequired[string]{Arg: "42"}
	cmd.Callback = func(v CommandWithArgWithRequired[string], w cli.Response, r *cli.Request) { cmd = v }

	cmd, err := cli.ConfigureHandler(cmd, "path", &cli.Request{})
	assert.NoError(t, err, "expected no error, since default value is already provided")
	assert.Equal(t, cmd.Arg, "42")
}

func TestConfigureHandler_envIntegration(t *testing.T) {
	cmd := CommandWithFlagWithEnv[string]{}
	const envName = "FLAGNAME"
	t.Run("no env var", func(t *testing.T) {
		testcase.UnsetEnv(t, envName)
		cmd, err := cli.ConfigureHandler(cmd, "path", &cli.Request{})
		assert.NoError(t, err)
		assert.Empty(t, cmd.Flag)
	})

	t.Run("with env var", func(t *testing.T) {
		testcase.SetEnv(t, envName, "val")

		cmd, err := cli.ConfigureHandler(cmd, "path", &cli.Request{})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Flag, "val")
	})
}

func TestConfigureHandler_enumIntegration(t *testing.T) {
	cmd := CommandWithFlagWithEnum{}

	t.Run("no enum value", func(t *testing.T) {
		cmd, err := cli.ConfigureHandler(cmd, "path", &cli.Request{Args: []string{}})
		assert.NoError(t, err)
		assert.Empty(t, cmd.Flag)
	})

	t.Run("with valid enum value", func(tt *testing.T) {
		t := testcase.NewT(tt)
		exp := random.Pick(t.Random, "foo", "bar", "baz")

		cmd, err := cli.ConfigureHandler(cmd, "path", &cli.Request{Args: []string{"-flag", exp}})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Flag, exp)
	})

	t.Run("with invalid enum value", func(t *testing.T) {
		_, err := cli.ConfigureHandler(cmd, "path", &cli.Request{Args: []string{"-flag", "invalid"}})

		assert.Error(t, err)

		var got errorkit.UserError
		assert.True(t, errors.As(err, &got))
		assert.Equal(t, got.ID, "enum-error")
	})
}

func TestConfigureHandler_unexportedStructField(t *testing.T) {
	req := &cli.Request{Args: []string{"-ef", "fooinput"}}
	h, err := cli.ConfigureHandler(&CommandWithUnexportedField{}, "", req)
	assert.NoError(t, err)
	assert.NotNil(t, h)
	assert.Equal(t, "fooinput", h.ExportedFlag)

	_ = h.unexportedFlag // we leave this undefined, not making it official that we support this.
}

type CommandWithUnexportedField struct {
	ExportedFlag   string `flag:"ef" env:"EF" default:"foo"`
	unexportedFlag string `flag:"uf" env:"UF" default:"bar"`
}

func (cmd CommandWithUnexportedField) ServeCLI(w cli.Response, r *cli.Request) {
	w.Write([]byte("Hello, world!"))
}
