package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"

	"go.llib.dev/frameless/internal/sandbox"
	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/osint"
	"go.llib.dev/frameless/pkg/iokit"
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

	cli.Main(context.Background(), &mux)
}

type FooCommandArgs struct {
	VeryDescriptiveArgumentName string
	NotSoDescriptiveArgName     bool `desc:"but the description here makes it descriptive"`
}

type FooCommand struct {
	A string `flag:"the-a,a" default:"val"   desc:"this is flag A"         opt:"T"`
	B bool   `flag:"the-b,b" default:"true"                                opt:"T"` // missing description
	C int    `flag:"c" required:"true"       desc:"this is flag C, not B"`
	D string `flag:"d" enum:"FOO,BAR,BAZ,"   desc:"this flag is an enum"   opt:"T"`

	Arg    string `arg:"0" desc:"something something" opt:"T"`
	OthArg int    `arg:"1" default:"42" opt:"T"`

	// Dependency is a dependency of the FooCommand, which is populated though traditional dependency injection.
	Dependency string
}

func (cmd FooCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	fmt.Fprintln(w, "hello")
}

type SubCommand struct{}

func (cmd SubCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
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
			request = testcase.Let(s, func(t *testcase.T) *cli.Request {
				return &cli.Request{}
			})
		)
		act := func(t *testcase.T) {
			mux.Get(t).ServeCLI(response.Get(t), request.Get(t))
		}

		callback := let.Var(s, func(t *testcase.T) func(w cli.ResponseWriter, r *cli.Request) {
			return func(w cli.ResponseWriter, r *cli.Request) {}
		})

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
					return StubServeCLIFunc(func(w cli.ResponseWriter, r *cli.Request) {
						callback.Get(t)(w, r)
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
				command.Set(t, StubServeCLIFunc(func(w cli.ResponseWriter, r *cli.Request) {
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
				command.Set(t, CommandE2E{Callback: func(h CommandE2E, w cli.ResponseWriter, r *cli.Request) {
					cmd = h
					fmt.Fprintln(w, "out")
					fmt.Fprintln(w.(cli.ErrorWriter).Stderr(), "errout")
				}})

				act(t)

				assert.Contains(t, response.Get(t).Out.String(), "out")
				assert.Contains(t, response.Get(t).Err.String(), "errout")

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

				command.Set(t, &CommandWithPointerReceiver{Callback: func(h *CommandWithPointerReceiver, w cli.ResponseWriter, r *cli.Request) {
					assert.NotNil(t, h)
					assert.NotEmpty(t, h.Flag)
					assert.NotEmpty(t, h.Arg)
					fmt.Fprint(w, "teapot")
				}})

				act(t)

				assert.Contains(t, response.Get(t).Out.String(), "teapot")
			})

			s.And("the command have flags", func(s *testcase.Spec) {
				var h = testcase.LetValue[*ExampleCommandWithFlag](s, nil)

				command.Let(s, func(t *testcase.T) cli.Handler {
					return ExampleCommandWithFlag{Callback: func(v ExampleCommandWithFlag, w cli.ResponseWriter, r *cli.Request) {
						h.Set(t, &v)
						callback.Get(t)(w, r)
						fmt.Fprintln(w, ExpCmdReply.Get(t))
					}}
				})

				s.And("one of the flag is a bool type", func(s *testcase.Spec) {
					var _ bool = ExampleCommandWithFlag{}.Flag5

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
					Callback: callback,
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
					Callback: callback,
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
					Callback: callback,
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
					Callback: callback,
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
					Callback: callback,
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
							Callback: func(v CommandWithFlagWithRequired[string], w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							},
						}
					})

					AndTheFlagTypeIs[string]{
						IsRequired: true,

						Act:      act,
						Callback: callback,
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
							Callback: func(v CommandWithFlagWithEnum, w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							},
						}
					})

					s.And("the input value is within the enum set", func(s *testcase.Spec) {
						exp := let.OneOf(s, "foo", "bar", "baz")

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
							assert.Contains(t, errOutput, "flag")
							assert.Contains(t, errOutput, "enum")
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
								assert.Contains(t, response.Get(t).Err.String(), v.String())
							}
						})
					})
				})
			})

			s.When("the command has argument(s)", func(s *testcase.Spec) {
				s.Context("optional", func(s *testcase.Spec) {
					s.Context("string", func(s *testcase.Spec) {
						h := testcase.LetValue[*CommandWithOptArg[string]](s, nil)

						command.Let(s, func(t *testcase.T) cli.Handler {
							return CommandWithOptArg[string]{Callback: func(v CommandWithOptArg[string], w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							}}
						})

						AndTheArgTypeIs[string]{
							Act:      act,
							Callback: callback,
							Response: response,
							Request:  request,
							Expected: func(t *testcase.T) string {
								return t.Random.String()
							},
							Got: func(t *testcase.T) string {
								return h.Get(t).Arg
							},
						}.OptionalArgSpec(s)
					})

					s.Context("int", func(s *testcase.Spec) {
						h := testcase.LetValue[*CommandWithOptArg[int]](s, nil)

						command.Let(s, func(t *testcase.T) cli.Handler {
							return CommandWithOptArg[int]{Callback: func(v CommandWithOptArg[int], w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							}}
						})

						AndTheArgTypeIs[int]{
							Act:      act,
							Callback: callback,
							Response: response,
							Request:  request,
							Expected: func(t *testcase.T) int {
								return t.Random.Int()
							},
							Got: func(t *testcase.T) int {
								return h.Get(t).Arg
							},
						}.OptionalArgSpec(s)
					})

					s.Context("bool", func(s *testcase.Spec) {
						h := testcase.LetValue[*CommandWithOptArg[bool]](s, nil)

						command.Let(s, func(t *testcase.T) cli.Handler {
							return CommandWithOptArg[bool]{Callback: func(v CommandWithOptArg[bool], w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							}}
						})

						AndTheArgTypeIs[bool]{
							Act:      act,
							Callback: callback,
							Response: response,
							Request:  request,
							Expected: func(t *testcase.T) bool {
								return t.Random.Bool()
							},
							Got: func(t *testcase.T) bool {
								return h.Get(t).Arg
							},
						}.OptionalArgSpec(s)
					})

					s.Context("along with a default", func(s *testcase.Spec) {
						h := testcase.LetValue[*CommandWithArgWithDefault](s, nil)

						command.Let(s, func(t *testcase.T) cli.Handler {
							return CommandWithArgWithDefault{Callback: func(v CommandWithArgWithDefault, w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							}}
						})

						AndTheArgTypeIs[string]{
							Default: "defval",

							Act:      act,
							Callback: callback,
							Response: response,
							Request:  request,
							Expected: func(t *testcase.T) string { return t.Random.String() },
							Got:      func(t *testcase.T) string { return h.Get(t).Arg },
						}.OptionalArgSpec(s)
					})
				})

				s.Context("required", func(s *testcase.Spec) {
					s.Context("string", func(s *testcase.Spec) {
						h := testcase.LetValue[*CommandWithReqArg[string]](s, nil)

						command.Let(s, func(t *testcase.T) cli.Handler {
							return CommandWithReqArg[string]{Callback: func(v CommandWithReqArg[string], w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							}}
						})

						AndTheArgTypeIs[string]{
							Act:        act,
							Callback:   callback,
							Response:   response,
							Request:    request,
							IsRequired: true,
							Expected: func(t *testcase.T) string {
								return t.Random.String()
							},
							Got: func(t *testcase.T) string {
								return h.Get(t).Arg
							},
						}.OptionalArgSpec(s)
					})

					s.Context("int", func(s *testcase.Spec) {
						h := testcase.LetValue[*CommandWithReqArg[int]](s, nil)

						command.Let(s, func(t *testcase.T) cli.Handler {
							return CommandWithReqArg[int]{Callback: func(v CommandWithReqArg[int], w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							}}
						})

						AndTheArgTypeIs[int]{
							Act:        act,
							Callback:   callback,
							Response:   response,
							Request:    request,
							IsRequired: true,
							Expected: func(t *testcase.T) int {
								return t.Random.Int()
							},
							Got: func(t *testcase.T) int {
								return h.Get(t).Arg
							},
						}.OptionalArgSpec(s)
					})

					s.Context("bool", func(s *testcase.Spec) {
						h := testcase.LetValue[*CommandWithReqArg[bool]](s, nil)

						command.Let(s, func(t *testcase.T) cli.Handler {
							return CommandWithReqArg[bool]{Callback: func(v CommandWithReqArg[bool], w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							}}
						})

						AndTheArgTypeIs[bool]{
							Act:        act,
							Callback:   callback,
							Response:   response,
							Request:    request,
							IsRequired: true,
							Expected: func(t *testcase.T) bool {
								return t.Random.Bool()
							},
							Got: func(t *testcase.T) bool {
								return h.Get(t).Arg
							},
						}.OptionalArgSpec(s)
					})
				})

				s.Context("which is mandatory/required", func(s *testcase.Spec) {
					h := testcase.Let(s, func(t *testcase.T) *CommandWithArgWithRequired[string] {
						return &CommandWithArgWithRequired[string]{}
					})

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithArgWithRequired[string]{Callback: func(v CommandWithArgWithRequired[string], w cli.ResponseWriter, r *cli.Request) {
							h.Set(t, &v)
							callback.Get(t)(w, r)
						}}
					})

					AndTheArgTypeIs[string]{
						IsRequired: true,

						Act:      act,
						Callback: callback,
						Response: response,
						Request:  request,
						Expected: func(t *testcase.T) string { return t.Random.String() },
						Got:      func(t *testcase.T) string { return h.Get(t).Arg },
					}.OptionalArgSpec(s)
				})

				s.And("the it has an enum limitation", func(s *testcase.Spec) {
					h := testcase.LetValue[*CommandWithOptArgWithEnum](s, nil)

					command.Let(s, func(t *testcase.T) cli.Handler {
						return CommandWithOptArgWithEnum{
							Callback: func(v CommandWithOptArgWithEnum, w cli.ResponseWriter, r *cli.Request) {
								h.Set(t, &v)
								callback.Get(t)(w, r)
							},
						}
					})

					s.And("the input value is within the enum set", func(s *testcase.Spec) {
						exp := let.OneOf(s, "foo", "bar", "baz")

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

							assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
						})

						s.Then("the error output will contain response", func(t *testcase.T) {
							act(t)

							assert.NotEmpty(t, response.Get(t).Err.String())
						})

						s.Then("the error ouput will be written", func(t *testcase.T) {
							act(t)

							errOutput := response.Get(t).Err.String()
							assert.NotEmpty(t, errOutput)

							assert.Contains(t, errOutput, "sage: "+commandName.Get(t))
							assert.AnyOf(t, func(a *assert.A) {
								a.Case(func(t testing.TB) { assert.Contains(t, errOutput, "qux") })
								a.Case(func(t testing.TB) { assert.Contains(t, errOutput, "quux") })
							})
						})

						s.Then("the the error response will mention valid enum values", func(t *testcase.T) {
							act(t)

							field, ok := reflectkit.TypeOf[CommandWithOptArgWithEnum]().FieldByName("Arg")
							assert.True(t, ok, "expected to find the struct value")
							vs, err := enum.ReflectValuesOfStructField(field)
							assert.NoError(t, err)
							assert.NotEmpty(t, vs)

							var _ string = CommandWithOptArgWithEnum{}.Arg // static check for Flag field type
							for _, v := range vs {
								assert.Contains(t, response.Get(t).Err.String(), v.String())
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

						assert.Contains(t, response.Get(t).Err.String(), unknownCommandName.Get(t),
							"expected that the unknown command name is mentioned")
					})

					s.Context("but when other commands are registered", func(s *testcase.Spec) {
						mux.Let(s, func(t *testcase.T) *cli.Mux {
							m := mux.Super(t)
							m.Handle("foo", StubServeCLIFunc(func(w cli.ResponseWriter, r *cli.Request) {}))
							m.Handle("bar", StubServeCLIFunc(func(w cli.ResponseWriter, r *cli.Request) {}))
							m.Handle("baz", StubServeCLIFunc(func(w cli.ResponseWriter, r *cli.Request) {}))
							return m
						})

						s.Then("available commands are suggested", func(t *testcase.T) {
							act(t)

							assert.Empty(t, response.Get(t).Out.String())
							assert.NotEmpty(t, response.Get(t).Err.String())

							const expectedToFindCommands = "expected that the available commands are listed"
							t.Log(response.Get(t).Err.String())
							assert.Contains(t, response.Get(t).Err.String(), "foo", expectedToFindCommands)
							assert.Contains(t, response.Get(t).Err.String(), "bar", expectedToFindCommands)
							assert.Contains(t, response.Get(t).Err.String(), "baz", expectedToFindCommands)
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
					assert.Contains(t, strings.ToLower(out), "usage")
				})
			}

			thenExitCodeIsOK(s)

			thenUsagePrintedOutToSTDOUT(s)

			s.Then("the help request is not interpreted as an unknown command", func(t *testcase.T) {
				act(t)

				assert.NotContains(t, strings.ToLower(response.Get(t).Out.String()), "unknown")
			})

			s.When("commands are registered", func(s *testcase.Spec) {
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
					assert.Contains(t, strings.ToLower(out), "commands")
					assert.Contains(t, out, "foo")
					assert.Contains(t, out, "e2e: "+CommandE2E{}.Summary())
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

						assert.Contains(t, out, "")
					})
				})
			})
		})
	})
}

func TestServeCLI(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		handler  = let.Var[cli.Handler](s, nil)
		response = let.Var(s, func(t *testcase.T) *cli.ResponseRecorder {
			return &cli.ResponseRecorder{}
		})
		request = let.Var(s, func(t *testcase.T) *cli.Request {
			return &cli.Request{}
		})
	)
	act := let.Act0(func(t *testcase.T) {
		cli.ServeCLI(handler.Get(t), response.Get(t), request.Get(t))
	})

	s.Test("cmd", func(t *testcase.T) {
		testcase.SetEnv(t, "FLAG3", "24")

		var configuredCommant CommandE2E
		cmd := CommandE2E{Callback: func(v CommandE2E, w cli.ResponseWriter, r *cli.Request) {
			configuredCommant = v
			data, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			_, _ = w.Write(data)
		}}

		const expBody = "foo/bar/baz"
		var w cli.ResponseRecorder
		cli.ServeCLI(cmd, &w, &cli.Request{
			Args: []string{
				"-str", "strstring",
				"-bool=true",
				"Hello, world!", "42", "true",
			},
			Body: strings.NewReader(expBody),
		})

		assert.Equal(t, cli.ExitCodeOK, w.Code, assert.Message(w.Err.String()))
		assert.Equal(t, expBody, w.Out.String())
		assert.Equal(t, configuredCommant.Arg1, "Hello, world!")
		assert.Equal(t, configuredCommant.Arg2, 42)
		assert.Equal(t, configuredCommant.Arg3, true)
		assert.Equal(t, configuredCommant.Flag1, "strstring")
		assert.Equal(t, configuredCommant.Flag2, "defval")
		assert.Equal(t, configuredCommant.Flag3, 24)
		assert.Equal(t, configuredCommant.Flag4, true)
	})

	s.Test("mux", func(t *testcase.T) {
		var mux cli.Mux

		testcase.SetEnv(t, "FLAG3", "24")

		var configuredCommant CommandE2E
		cmd := CommandE2E{Callback: func(v CommandE2E, w cli.ResponseWriter, r *cli.Request) {
			configuredCommant = v
			data, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			_, _ = w.Write(data)
		}}

		mux.Handle("cmd", cmd)

		const expBody = "foo/bar/baz"
		var w cli.ResponseRecorder
		cli.ServeCLI(&mux, &w, &cli.Request{
			Args: []string{"cmd",
				"-str", "strstring",
				"-bool=true",
				"Hello, world!", "42", "true",
			},
			Body: strings.NewReader(expBody),
		})

		assert.Equal(t, cli.ExitCodeOK, w.Code, assert.Message(w.Err.String()))
		assert.Equal(t, expBody, w.Out.String())
		assert.Equal(t, configuredCommant.Arg1, "Hello, world!")
		assert.Equal(t, configuredCommant.Arg2, 42)
		assert.Equal(t, configuredCommant.Arg3, true)
		assert.Equal(t, configuredCommant.Flag1, "strstring")
		assert.Equal(t, configuredCommant.Flag2, "defval")
		assert.Equal(t, configuredCommant.Flag3, 24)
		assert.Equal(t, configuredCommant.Flag4, true)
	})

	s.Context("dependency", func(s *testcase.Spec) {
		type Dependency struct {
			Env  string `env:"ENV_VAL"`
			Flag string `flag:"flag_val"`
			Arg  string `arg:"0"`
		}

		var dep = let.Var[Dependency](s, nil)

		cmd := let.Var(s, func(t *testcase.T) CommandWithTypedDependency[Dependency] {
			return CommandWithTypedDependency[Dependency]{
				Callback: func(v CommandWithTypedDependency[Dependency], w cli.ResponseWriter, r *cli.Request) {
					dep.Set(t, v.D)
				},
			}
		})

		s.Test("ok", func(t *testcase.T) {
			var (
				argV  = t.Random.String()
				flagV = t.Random.String()
				envV  = t.Random.String()
			)
			testcase.SetEnv(t, "ENV_VAL", envV)

			r := &cli.Request{Args: []string{"-flag_val", flagV, argV}}
			w := &cli.ResponseRecorder{}
			cli.ServeCLI(cmd.Get(t), w, r)

			assert.Equal(t, cli.ExitCodeOK, w.Code,
				assert.MessageF("%s%s", w.Out.String(), w.Err.String()))

			assert.Equal(t, dep.Get(t).Env, envV)
			assert.Equal(t, dep.Get(t).Flag, flagV)
			assert.Equal(t, dep.Get(t).Arg, argV)
		})
	})

	s.Test("flag with accidental empty alias name", func(t *testcase.T) {
		type D struct {
			V string `flag:"v,"`
		}
		var d D

		var h = CommandWithTypedDependency[D]{
			Callback: func(v CommandWithTypedDependency[D], w cli.ResponseWriter, r *cli.Request) {
				d = v.D
			},
		}

		exp := t.Random.String()

		w := &cli.ResponseRecorder{}
		r := &cli.Request{Args: []string{"-v", exp}}
		cli.ServeCLI(h, w, r)
		assert.Equal(t, d.V, exp)

		w = &cli.ResponseRecorder{}
		r = &cli.Request{Args: []string{"--help"}}
		cli.ServeCLI(h, w, r)
		assert.NotContains(t, w.Out.String(), "-\n")
		assert.NotContains(t, w.Err.String(), "-\n")

	})

	s.Test("flag with leading dash", func(t *testcase.T) {
		type D struct {
			V string `flag:"-v"`
			B string `flag:"--b"`
		}
		var d D

		var h = CommandWithTypedDependency[D]{
			Callback: func(v CommandWithTypedDependency[D], w cli.ResponseWriter, r *cli.Request) {
				d = v.D
			},
		}

		expV := t.Random.String()
		expB := t.Random.String()

		w := &cli.ResponseRecorder{}
		r := &cli.Request{Args: []string{"-v", expV, "-b", expB}}
		cli.ServeCLI(h, w, r)
		assert.Equal(t, d.V, expV)
		assert.Equal(t, d.B, expB)
	})

	s.Context("validation", func(s *testcase.Spec) {
		type Dependency struct {
			XYZ string `flag:"xyz" len:"5<"`
		}

		var dep = let.Var[Dependency](s, nil)

		var handlerIsCalled = let.VarOf(s, false)
		cmd := let.Var(s, func(t *testcase.T) CommandWithTypedDependency[Dependency] {
			return CommandWithTypedDependency[Dependency]{
				Callback: func(v CommandWithTypedDependency[Dependency], w cli.ResponseWriter, r *cli.Request) {
					handlerIsCalled.Set(t, true)
					dep.Set(t, v.D)
				},
			}
		})
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return cmd.Get(t)
		})

		val := let.Var[string](s, nil)

		request.Let(s, func(t *testcase.T) *cli.Request {
			req := request.Super(t)
			req.Args = append(req.Args, "-xyz", val.Get(t))
			return req
		})

		s.When("validation pass", func(s *testcase.Spec) {
			val.Let(s, let.StringNC(s, 10, random.CharsetAlpha()).Get)

			s.Then("handler is called", func(t *testcase.T) {
				act(t)

				assert.True(t, handlerIsCalled.Get(t))
				assert.Equal(t, dep.Get(t).XYZ, val.Get(t))
			})
		})

		s.When("validation detects an issue", func(s *testcase.Spec) {
			val.Let(s, let.StringNC(s, 3, random.CharsetAlpha()).Get)

			s.Then("handler is not called", func(t *testcase.T) {
				act(t)

				assert.False(t, handlerIsCalled.Get(t))
			})

			s.Then("error message returned", func(t *testcase.T) {
				act(t)
				assert.Contains(t, strings.ToLower(response.Get(t).Err.String()), "invalid")
				assert.Contains(t, response.Get(t).Err.String(), "xyz")
				assert.Contains(t, response.Get(t).Err.String(), "5<")
			})
		})
	})
}

type CommandWithTypedDependency[Dependency any] struct {
	Callback[CommandWithTypedDependency[Dependency]]

	D Dependency
}

func (cmd CommandWithTypedDependency[Dependency]) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type AndTheFlagTypeIs[T any] struct {
	Desc     string
	Act      func(t *testcase.T)
	Response testcase.Var[*cli.ResponseRecorder]
	Request  testcase.Var[*cli.Request]
	Callback testcase.Var[func(w cli.ResponseWriter, r *cli.Request)]

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

		gotRequest := let.Var[*cli.Request](s, nil)
		gotResponse := let.Var[cli.ResponseWriter](s, nil)

		c.Callback.Let(s, func(t *testcase.T) func(w cli.ResponseWriter, r *cli.Request) {
			return func(w cli.ResponseWriter, r *cli.Request) {
				gotResponse.Set(t, w)
				gotRequest.Set(t, r)
			}
		})

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

				assert.NotContains(t, gotRequest.Get(t).Args, c.Flag)
				assert.NotContains(t, gotRequest.Get(t).Args, raw.Get(t))
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
					assert.NotContains(t, arg, c.Flag)
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
	Callback testcase.Var[func(w cli.ResponseWriter, r *cli.Request)]
	Response testcase.Var[*cli.ResponseRecorder]
	Request  testcase.Var[*cli.Request]

	Default T

	Expected testcase.VarInit[T]
	Got      func(t *testcase.T) T

	IsRequired bool
}

func (c AndTheArgTypeIs[T]) name() string {
	return reflectkit.SymbolicName(reflectkit.TypeOf[T]())
}

func (c AndTheArgTypeIs[T]) OptionalArgSpec(s *testcase.Spec) {
	s.And("the ARG value is provided as a "+c.name()+" value", func(s *testcase.Spec) {
		exp := testcase.Let(s, c.Expected)

		gotRequest := let.Var[*cli.Request](s, nil)
		gotResponse := let.Var[cli.ResponseWriter](s, nil)

		c.Callback.Let(s, func(t *testcase.T) func(w cli.ResponseWriter, r *cli.Request) {
			return func(w cli.ResponseWriter, r *cli.Request) {
				gotResponse.Set(t, w)
				gotRequest.Set(t, r)
			}
		})

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

			assert.NotContains(t, gotRequest.Get(t).Args, raw.Get(t))
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

type StubServeCLIFunc func(w cli.ResponseWriter, r *cli.Request)

func (cmd StubServeCLIFunc) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
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

type Callback[T any] func(v T, w cli.ResponseWriter, r *cli.Request)

func (fn Callback[T]) Call(v T, w cli.ResponseWriter, r *cli.Request) {
	if fn != nil {
		fn(v, w, r)
	}
}

type CommandE2E struct {
	Callback[CommandE2E]

	Flag1 string `flag:"str,rts," desc:"flag1 desc"`
	Flag2 string `flag:"strwd,dwrts" default:"defval"`
	Flag3 int    `flag:"int" env:"FLAG3"`
	Flag4 bool   `flag:"bool"`
	Flag5 bool   `flag:"sbool"`
	Flag6 bool   `flag:"fbool"`

	Arg1 string `arg:"0"`
	Arg2 int    `arg:"1"`
	Arg3 bool   `arg:"2"`

	Env1 string `desc:"env-1" env:"ENV1" default:"1vne"`
	Env2 string `desc:"env-1" env:"ENV2,ENV22" default:"2vne"`
}

func (cmd CommandE2E) Summary() string { return "E2E command summary" }

func (cmd CommandE2E) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithPointerReceiver struct {
	Callback[*CommandWithPointerReceiver]
	Flag string `flag:"flag"`
	Arg  string `arg:"0"`
}

func (cmd *CommandWithPointerReceiver) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type ExampleCommandWithFlag struct {
	Callback[ExampleCommandWithFlag]
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

func (cmd ExampleCommandWithFlag) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithFlagWithRequired[T any] struct {
	Callback[CommandWithFlagWithRequired[T]]
	Flag T `flag:"flag" required:"true"`
}

func (cmd CommandWithFlagWithRequired[T]) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithFlagWithEnv[T any] struct {
	Callback[CommandWithFlagWithEnv[T]]
	Flag T `flag:"flag" env:"FLAGNAME"`
}

func (cmd CommandWithFlagWithEnv[T]) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithFlagWithEnum struct {
	Callback[CommandWithFlagWithEnum]
	Flag string `flag:"flag" enum:"foo,bar,baz," opt:"T"`
}

func (cmd CommandWithFlagWithEnum) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithOptArg[T any] struct {
	Callback[CommandWithOptArg[T]]
	Arg T `arg:"0" opt:"true"`
}

func (cmd CommandWithOptArg[T]) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithReqArg[T any] struct {
	Callback[CommandWithReqArg[T]]
	Arg T `arg:"0"`
}

func (cmd CommandWithReqArg[T]) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithArgWithDefault struct {
	Callback[CommandWithArgWithDefault]
	Arg string `arg:"0" default:"defval"` // anything with a default is already optional
}

func (cmd CommandWithArgWithDefault) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithArgWithRequired[T any] struct {
	Callback[CommandWithArgWithRequired[T]]
	Arg T `arg:"0" required:"1"`
}

func (cmd CommandWithArgWithRequired[T]) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithOptArgWithEnum struct {
	Callback[CommandWithOptArgWithEnum]
	Arg string `arg:"0" enum:"foo,bar,baz,"`
}

func (cmd CommandWithOptArgWithEnum) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

func TestConfigureHandler(t *testing.T) {
	req := &cli.Request{Args: []string{
		/* flags */ "-str", "foo", "-int", "42", "-bool=true", "-sbool", "-fbool=0",
		/* args */ "hello-world", "42", "True",
	}}
	var cmd CommandE2E

	err := cli.ConfigureHandler(&cmd, req)
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

type CommandWithUsageSupport struct{}

func (CommandWithUsageSupport) Usage(path string) (string, error) {
	return fmt.Sprintf("Custom Usage Message: %s", path), nil
}

func (CommandWithUsageSupport) ServeCLI(w cli.ResponseWriter, r *cli.Request) {}

type CommandWithDependency struct {
	Flag1 string `flag:"flag1" default:"val" desc:"this is flag A"`
	Flag2 bool   `flag:"flag2" default:"true"`
	Flag3 int    `flag:"othflag" required:"true" desc:"this is flag C, not B"`

	Arg1 string `arg:"0" desc:"something something"`
	Arg2 int    `arg:"1" default:"42" desc:"something something else"`

	// Dependency is a dependency of the FooCommand, which is populated though traditional dependency injection.
	Dependency any
}

func (CommandWithDependency) ServeCLI(w cli.ResponseWriter, r *cli.Request) {}

func Example_dependencyInjection() {
	cli.Main(context.Background(), CommandWithDependency{
		Dependency: "important dependency that I need as part of the ServeCLI call",
	})
}

func TestConfigureHandler_requiredFlag_defaultValueDependencyInjected(t *testing.T) {
	cmd := CommandWithFlagWithRequired[string]{Flag: "42"}
	cmd.Callback = func(v CommandWithFlagWithRequired[string], w cli.ResponseWriter, r *cli.Request) { cmd = v }

	err := cli.ConfigureHandler(&cmd, &cli.Request{})
	assert.NoError(t, err, "expected no error, since default value is already provided")
	assert.Equal(t, cmd.Flag, "42")
}

func TestConfigureHandler_requiredArg_injextedDefaultValue(t *testing.T) {
	cmd := CommandWithArgWithRequired[string]{Arg: "42"}
	cmd.Callback = func(v CommandWithArgWithRequired[string], w cli.ResponseWriter, r *cli.Request) { cmd = v }

	err := cli.ConfigureHandler(&cmd, &cli.Request{})
	assert.NoError(t, err, "expected no error, since default value is already provided")
	assert.Equal(t, cmd.Arg, "42")
}

func TestConfigureHandler_envIntegration(t *testing.T) {
	cmd := CommandWithFlagWithEnv[string]{}
	const envName = "FLAGNAME"
	t.Run("no env var", func(t *testing.T) {
		testcase.UnsetEnv(t, envName)
		err := cli.ConfigureHandler(&cmd, &cli.Request{})
		assert.NoError(t, err)
		assert.Empty(t, cmd.Flag)
	})

	t.Run("with env var", func(t *testing.T) {
		testcase.SetEnv(t, envName, "val")

		err := cli.ConfigureHandler(&cmd, &cli.Request{})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Flag, "val")
	})
}

func TestConfigureHandler_enumIntegration(t *testing.T) {
	cmd := CommandWithFlagWithEnum{}

	t.Run("no enum value", func(t *testing.T) {
		err := cli.ConfigureHandler(&cmd, &cli.Request{Args: []string{}})
		assert.NoError(t, err)
		assert.Empty(t, cmd.Flag)
	})

	t.Run("with valid enum value", func(tt *testing.T) {
		t := testcase.NewT(tt)
		exp := random.Pick(t.Random, "foo", "bar", "baz")

		err := cli.ConfigureHandler(&cmd, &cli.Request{Args: []string{"-flag", exp}})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Flag, exp)
	})

	t.Run("with invalid enum value", func(t *testing.T) {
		err := cli.ConfigureHandler(&cmd, &cli.Request{Args: []string{"-flag", "invalid"}})
		assert.Error(t, err)

		var got errorkit.UserError
		assert.True(t, errors.As(err, &got))
		assert.Equal(t, got.Code, "enum-error")
	})
}

func TestConfigureHandler_unexportedStructField(t *testing.T) {
	req := &cli.Request{Args: []string{"-ef", "fooinput"}}
	var h CommandWithUnexportedField
	err := cli.ConfigureHandler(&h, req)
	assert.NoError(t, err)
	assert.NotNil(t, h)
	assert.Equal(t, "fooinput", h.ExportedFlag)

	_ = h.unexportedFlag // we leave this undefined, not making it official that we support this.
}

type CommandWithUnexportedField struct {
	ExportedFlag   string `flag:"ef" env:"EF" default:"foo"`
	unexportedFlag string `flag:"uf" env:"UF" default:"bar"`
}

func (cmd CommandWithUnexportedField) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	w.Write([]byte("Hello, world!"))
}

func Test_main(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		handlerExitCode     = let.IntB(s, 0, 100)
		handlerResponseBody = let.String(s)
	)

	var (
		ctx     = let.Context(s)
		handler = let.Var(s, func(t *testcase.T) cli.Handler {
			return cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
				w.ExitCode(handlerExitCode.Get(t))
				iokit.WriteAll(w, []byte(handlerResponseBody.Get(t)))
			})
		})
	)
	act := let.Act(func(t *testcase.T) sandbox.O {
		return sandbox.Run(func() {
			cli.Main(ctx.Get(t), handler.Get(t))
		})
	})

	code := let.VarOf[*int](s, nil)
	s.Before(func(t *testcase.T) {
		osint.StubExit(t, func(exitCode int) {
			code.Set(t, &exitCode)
			runtime.Goexit() // to mimic os.Exit(code)
		})
	})

	stderr := let.Var(s, func(t *testcase.T) io.Writer {
		return &bytes.Buffer{}
	})
	_ = stderr

	//TODO: finish me up

	s.Then("handler requested exit code is propagated as app os exit code", func(t *testcase.T) {
		o := act(t)
		assert.True(t, o.Goexit)
		assert.NotNil(t, code.Get(t))
		assert.Equal(t, *code.Get(t), handlerExitCode.Get(t))
	})

	s.Then("handler response body is propagated towards STDOUT", func(t *testcase.T) {

	})

}
