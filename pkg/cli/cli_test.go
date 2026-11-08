package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/internal/sandbox"
	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/osint"
	"go.llib.dev/frameless/pkg/internal/signalint"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func Example() {
	var mux cli.ServeMux

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

func TestServeMux(t *testing.T) {
	s := testcase.NewSpec(t)

	mux := testcase.Let(s, func(t *testcase.T) *cli.ServeMux {
		return &cli.ServeMux{}
	})

	s.Describe("#ServeCLI", func(s *testcase.Spec) {
		var (
			response = testcase.Let(s, func(t *testcase.T) *cli.ResponseRecorder {
				return &cli.ResponseRecorder{}
			})
			request = testcase.Let(s, func(t *testcase.T) *cli.Request {
				return &cli.Request{
					Args: []string{},
					Body: &bytes.Buffer{},
				}
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

			mux.Let(s, func(t *testcase.T) *cli.ServeMux {
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
					lastRequest    *cli.Request
					expectedOutput = t.Random.String()
				)
				command.Set(t, StubServeCLIFunc(func(w cli.ResponseWriter, r *cli.Request) {
					lastRequest = r
					w.ExitCode(42)
					w.Write([]byte(expectedOutput))
				}))

				act(t)

				assert.NotNil(t, lastRequest)
				assert.Equal(t, response.Get(t).Out.String(), expectedOutput)
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

							assert.MatchRegexp(t, errOutput, `sage: [^\s]+ `+regexp.QuoteMeta(commandName.Get(t)))
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
						mux.Let(s, func(t *testcase.T) *cli.ServeMux {
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

			s.When("summary is provided", func(s *testcase.Spec) {
				muxSummary := let.UUID(s)

				mux.Let(s, func(t *testcase.T) *cli.ServeMux {
					m := mux.Super(t)
					m2 := m.WithSummary(muxSummary.Get(t))
					if t.Random.Bool() { // builder style
						m = m2
					}
					return m
				})

				thenExitCodeIsOK(s)

				thenUsagePrintedOutToSTDOUT(s)

				s.Then("help message contains the summary", func(t *testcase.T) {
					act(t)

					assert.Contains(t, string(response.Get(t).CombinedOutput()), muxSummary.Get(t))
				})

				s.And("a sub mux is mounted", func(s *testcase.Spec) {
					subName := let.Var(s, makePattern)
					subSummary := let.Var(s, func(t *testcase.T) string {
						return random.Unique(t.Random.UUID, muxSummary.Get(t))
					})

					s.Before(func(t *testcase.T) {
						mux.Get(t).Sub(subName.Get(t)).
							WithSummary(subSummary.Get(t))
					})

					s.Then("help message contains the summary of the main mux", func(t *testcase.T) {
						act(t)

						assert.Contains(t, string(response.Get(t).CombinedOutput()), muxSummary.Get(t))
					})

					s.Then("help message contains the summary of the sub mux within the command section", func(t *testcase.T) {
						act(t)

						exp := fmt.Sprintf("- %s: %s", subName.Get(t), subSummary.Get(t))
						out := string(response.Get(t).CombinedOutput())
						assert.Contains(t, out, exp)
					})

					s.Then("the sub mux help message don't contain the summary of the outer mux", func(t *testcase.T) {
						request.Get(t).Args = []string{subName.Get(t), "--help"}
						mux.Get(t).ServeCLI(response.Get(t), request.Get(t))

						out := string(response.Get(t).CombinedOutput())
						assert.Contains(t, out, subSummary.Get(t))
						assert.NotContains(t, out, muxSummary.Get(t))
					})
				})

				s.And("description is also provided", func(s *testcase.Spec) {
					muxDescription := let.Var(s, func(t *testcase.T) string {
						return random.Unique(t.Random.UUID, muxSummary.Get(t))
					})

					mux.Let(s, func(t *testcase.T) *cli.ServeMux {
						return mux.Super(t).WithDescription(muxDescription.Get(t))
					})

					thenExitCodeIsOK(s)

					thenUsagePrintedOutToSTDOUT(s)

					s.Then("help message contains the summary", func(t *testcase.T) {
						act(t)

						assert.Contains(t, string(response.Get(t).CombinedOutput()), muxSummary.Get(t))
					})

					s.Then("help message contains the description", func(t *testcase.T) {
						act(t)

						assert.Contains(t, string(response.Get(t).CombinedOutput()), muxDescription.Get(t))
					})

					s.Then("in the help message, the usage is before the description", func(t *testcase.T) {
						act(t)

						out := string(response.Get(t).CombinedOutput())
						indexSummary := strings.Index(out, muxSummary.Get(t))
						indexDescription := strings.Index(out, muxDescription.Get(t))
						assert.Assert(t, indexSummary < indexDescription, "expected that summary is before the description")
					})
				})
			})

			s.When("description is provided", func(s *testcase.Spec) {
				muxDesc := let.UUID(s)

				mux.Let(s, func(t *testcase.T) *cli.ServeMux {
					m := mux.Super(t)
					m2 := m.WithDescription(muxDesc.Get(t))
					if t.Random.Bool() { // builder style
						m = m2
					}
					return m
				})

				thenExitCodeIsOK(s)

				thenUsagePrintedOutToSTDOUT(s)

				s.Then("help message contains the description", func(t *testcase.T) {
					act(t)

					assert.Contains(t, string(response.Get(t).CombinedOutput()), muxDesc.Get(t))
				})

				s.And("a sub mux is mounted", func(s *testcase.Spec) {
					name := let.Var(s, makePattern)

					subDesc := let.Var(s, func(t *testcase.T) string {
						return random.Unique(t.Random.UUID, muxDesc.Get(t))
					})

					s.Before(func(t *testcase.T) {
						sub := mux.Get(t).Sub(name.Get(t))
						sub.WithDescription(subDesc.Get(t))
					})

					s.Then("help message contains the description", func(t *testcase.T) {
						act(t)

						assert.Contains(t, string(response.Get(t).CombinedOutput()), muxDesc.Get(t))
					})

					s.Then("the sub mux help message don't contain the description of the outer mux", func(t *testcase.T) {
						request.Get(t).Args = []string{name.Get(t), "--help"}
						mux.Get(t).ServeCLI(response.Get(t), request.Get(t))

						out := string(response.Get(t).CombinedOutput())
						assert.Contains(t, out, subDesc.Get(t))
						assert.NotContains(t, out, muxDesc.Get(t))
					})
				})
			})

			s.When("commands are registered", func(s *testcase.Spec) {
				mux.Let(s, func(t *testcase.T) *cli.ServeMux {
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
						assert.Contains(t, out, "")
					})
				})
			})

			s.When("commands are registered with various summary", func(s *testcase.Spec) {
				commands := let.Var(s, func(t *testcase.T) map[string]CommandWithHelpText {
					var summaries, descriptions []string
					cmds := random.Slice(t.Random.IntBetween(3, 7), func() CommandWithHelpText {
						summary := random.Unique(t.Random.UUID, summaries...)
						summaries = append(summaries, summary)

						description := random.Unique(t.Random.UUID, descriptions...)
						descriptions = append(summaries, description)

						return CommandWithHelpText{
							summary:     summary,
							description: description,
						}
					})

					var commands = map[string]CommandWithHelpText{}
					for _, cmd := range cmds {
						name := random.Unique(t.Random.UUID,
							slicekit.Merge(summaries, descriptions, mapkit.Keys(commands))...)

						commands[name] = cmd
					}
					return commands
				})

				mux.Let(s, func(t *testcase.T) *cli.ServeMux {
					m := mux.Super(t)
					for name, cmd := range commands.Get(t) {
						m.Handle(name, cmd)
					}
					return m
				})

				thenExitCodeIsOK(s)

				thenUsagePrintedOutToSTDOUT(s)

				s.Then("commands with their unique summary are listed", func(t *testcase.T) {
					act(t)

					out := string(response.Get(t).CombinedOutput())
					assert.NotEmpty(t, out)

					for name, cmd := range commands.Get(t) {
						assert.Contains(t, out, fmt.Sprintf("- %s: %s", name, cmd.summary))
						assert.NotContains(t, out, cmd.description)
					}
				})

				s.And("and command is specified before the help flag", func(s *testcase.Spec) {
					cmdName := let.Var(s, func(t *testcase.T) string {
						return random.Pick(t.Random, mapkit.Keys(commands.Get(t))...)
					})

					request.Let(s, func(t *testcase.T) *cli.Request {
						r := request.Super(t)
						slicekit.Unshift(&r.Args, cmdName.Get(t))
						return r
					})

					thenExitCodeIsOK(s)

					thenUsagePrintedOutToSTDOUT(s)

					s.Then("the command specific documentation is printed out", func(t *testcase.T) {
						act(t)

						out := string(response.Get(t).CombinedOutput())
						cmd := commands.Get(t)[cmdName.Get(t)]
						assert.Contains(t, out, cmdName.Get(t))
						assert.Contains(t, out, cmd.summary)
						assert.Contains(t, out, cmd.description)
					})
				})
			})
		})
	})

	s.Describe("#Sub", func(s *testcase.Spec) {
		var (
			pattern = let.Var(s, makePattern)
		)
		act := func(t *testcase.T) *cli.ServeMux {
			return mux.Get(t).Sub(pattern.Get(t))
		}

		s.Then("on initial call, it returns a new Mux", func(t *testcase.T) {
			sub := act(t)
			assert.NotNil(t, sub)
			assert.NotEqual(t, mux.Get(t), sub)
		})

		s.Then("on repeated call, it returns the same Mux", func(t *testcase.T) {
			sub := act(t)

			t.Random.Repeat(3, 7, func() {
				assert.Equal(t, sub, act(t))
			})
		})

		s.Then("mux help contains sub mux command", func(t *testcase.T) {
			act(t)

			rr := &cli.ResponseRecorder{}
			req := &cli.Request{
				Args: []string{"--help"},
				Body: &bytes.Buffer{},
			}
			mux.Get(t).ServeCLI(rr, req)

			assert.Contains(t, string(rr.CombinedOutput()), pattern.Get(t))
		})

		s.Then("sub-mux help message can be requested", func(t *testcase.T) {
			act(t)

			rr := &cli.ResponseRecorder{}
			req := &cli.Request{
				Args: []string{pattern.Get(t), "--help"},
				Body: &bytes.Buffer{},
			}
			mux.Get(t).ServeCLI(rr, req)

			assert.MatchRegexp(t, string(rr.CombinedOutput()), `Usage: [^\s]+ `+regexp.QuoteMeta(pattern.Get(t)))
		})

		s.When("the sub-mux has a handler registered", func(s *testcase.Spec) {
			sub := let.Var(s, func(t *testcase.T) *cli.ServeMux {
				return act(t)
			})

			var commandName = let.Var(s, makePattern)

			s.Before(func(t *testcase.T) {
				sub.Get(t).Handle(commandName.Get(t), CommandE2E{})
			})

			s.Then("command is listed in the help message of the sub command", func(t *testcase.T) {
				rr := &cli.ResponseRecorder{}
				req := &cli.Request{
					Args: []string{"--help"},
					Body: &bytes.Buffer{},
				}
				sub.Get(t).ServeCLI(rr, req)

				assert.Contains(t, string(rr.CombinedOutput()), commandName.Get(t))
			})
		})
	})
}

type CommandWithHelpText struct {
	summary     string
	description string
}

func (cmd CommandWithHelpText) Summary() string { return cmd.summary }

func (cmd CommandWithHelpText) Description() string { return cmd.description }

func (cmd CommandWithHelpText) ServeCLI(w cli.ResponseWriter, r *cli.Request) {}

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
		var mux cli.ServeMux

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
				flagV = t.Random.StringNC(5, random.CharsetAlpha()+random.CharsetDigit())
				envV  = t.Random.String()
			)
			testcase.SetEnv(t, "ENV_VAL", envV)

			r := &cli.Request{Args: []string{"-flag_val", flagV, "--", argV}}
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

	s.When("two input fields of the command define the same flag", func(s *testcase.Spec) {
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return CommandWithFlagCollision{}
		})

		s.Then("it panics with the flag and the fields which define it, as it is a programming error", func(t *testcase.T) {
			out := assert.Panic(t, func() { act(t) })

			msg := fmt.Sprintf("%v", out)
			assert.Contains(t, msg, "-host")
			assert.Contains(t, msg, "Primary.Host")
			assert.Contains(t, msg, "Replica.Host")
		})
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

	s.Context("default tag", func(s *testcase.Spec) {
		const envKey = "CLI_TEST_DEFAULT_TAG_ENV" // the env tag of CommandWithDefaultTags.Env

		var (
			got = let.VarOf[*CommandWithDefaultTags](s, nil)
			cmd = let.Var(s, func(t *testcase.T) CommandWithDefaultTags {
				return CommandWithDefaultTags{
					Callback: func(v CommandWithDefaultTags, w cli.ResponseWriter, r *cli.Request) {
						got.Set(t, &v)
					},
				}
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return cmd.Get(t)
		})

		s.Before(func(t *testcase.T) {
			testcase.UnsetEnv(t, envKey)
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("default values are used for the inputs the user didn't provide", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Flag, "flag-default")
			assert.Equal(t, got.Get(t).Bool, true)
			assert.Equal(t, got.Get(t).Env, "env-default")
			assert.Equal(t, got.Get(t).Arg, "arg-default")
		})

		s.When("the user provides the inputs", func(s *testcase.Spec) {
			var (
				flagVal = let.StringNC(s, 8, random.CharsetAlpha())
				envVal  = let.StringNC(s, 8, random.CharsetAlpha())
				argVal  = let.StringNC(s, 8, random.CharsetAlpha())
			)
			request.Let(s, func(t *testcase.T) *cli.Request {
				r := request.Super(t)
				r.Args = append(r.Args, "-flag", flagVal.Get(t), "-bool=false", argVal.Get(t))
				return r
			})
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, envKey, envVal.Get(t))
			})

			s.Then("the user input takes precedence over the default tag values", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Flag, flagVal.Get(t))
				assert.Equal(t, got.Get(t).Bool, false)
				assert.Equal(t, got.Get(t).Env, envVal.Get(t))
				assert.Equal(t, got.Get(t).Arg, argVal.Get(t))
			})
		})

		s.When("the registered handler has a dependency", func(s *testcase.Spec) {
			dependency := let.StringNC(s, 8, random.CharsetAlpha())

			cmd.Let(s, func(t *testcase.T) CommandWithDefaultTags {
				c := cmd.Super(t)
				c.Dependency = dependency.Get(t)
				return c
			})

			s.Then("the dependency is kept, and the inputs get their default values", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Dependency, dependency.Get(t))
				assert.Equal(t, got.Get(t).Flag, "flag-default")
				assert.Equal(t, got.Get(t).Bool, true)
				assert.Equal(t, got.Get(t).Env, "env-default")
				assert.Equal(t, got.Get(t).Arg, "arg-default")
			})
		})

		s.When("the registered handler has only some of its inputs pre-set", func(s *testcase.Spec) {
			preFlag := let.StringNC(s, 8, random.CharsetAlpha())

			cmd.Let(s, func(t *testcase.T) CommandWithDefaultTags {
				c := cmd.Super(t)
				c.Flag = preFlag.Get(t)
				return c
			})

			s.Then("the pre-set input is kept, and the rest of the inputs get their default values", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Flag, preFlag.Get(t))
				assert.Equal(t, got.Get(t).Bool, true)
				assert.Equal(t, got.Get(t).Env, "env-default")
				assert.Equal(t, got.Get(t).Arg, "arg-default")
			})
		})

		s.When("an input is pre-set to its zero value, while its default tag has a different value", func(s *testcase.Spec) {
			cmd.Let(s, func(t *testcase.T) CommandWithDefaultTags {
				c := cmd.Super(t)
				c.Bool = false
				return c
			})

			s.Then("the default value is used, since a zero value can't be told apart from an unset one", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Bool, true)
			})

			s.And("the user sets the input to its zero value explicitly", func(s *testcase.Spec) {
				request.Let(s, func(t *testcase.T) *cli.Request {
					r := request.Super(t)
					r.Args = append(r.Args, "-bool=false")
					return r
				})

				s.Then("the user input is used instead of the default value", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).Bool, false)
				})
			})
		})

		s.When("the registered handler has pre-set values", func(s *testcase.Spec) {
			var (
				preFlag = let.StringNC(s, 8, random.CharsetAlpha())
				preEnv  = let.StringNC(s, 8, random.CharsetAlpha())
				preArg  = let.StringNC(s, 8, random.CharsetAlpha())
			)
			cmd.Let(s, func(t *testcase.T) CommandWithDefaultTags {
				c := cmd.Super(t)
				c.Flag = preFlag.Get(t)
				c.Env = preEnv.Get(t)
				c.Arg = preArg.Get(t)
				return c
			})

			s.Then("the pre-set values are kept instead of being overwritten by the default tag", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Flag, preFlag.Get(t))
				assert.Equal(t, got.Get(t).Env, preEnv.Get(t))
				assert.Equal(t, got.Get(t).Arg, preArg.Get(t))
				assert.Equal(t, got.Get(t).Bool, true, assert.Message("the input that wasn't pre-set should get its default value"))
			})

			s.And("the user provides the inputs", func(s *testcase.Spec) {
				var (
					flagVal = let.StringNC(s, 8, random.CharsetAlpha())
					envVal  = let.StringNC(s, 8, random.CharsetAlpha())
					argVal  = let.StringNC(s, 8, random.CharsetAlpha())
				)
				request.Let(s, func(t *testcase.T) *cli.Request {
					r := request.Super(t)
					r.Args = append(r.Args, "-flag", flagVal.Get(t), argVal.Get(t))
					return r
				})
				s.Before(func(t *testcase.T) {
					testcase.SetEnv(t, envKey, envVal.Get(t))
				})

				s.Then("the user input takes precedence over the pre-set values", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).Flag, flagVal.Get(t))
					assert.Equal(t, got.Get(t).Env, envVal.Get(t))
					assert.Equal(t, got.Get(t).Arg, argVal.Get(t))
				})
			})
		})
	})

	s.Context("bool input with a true default tag", func(s *testcase.Spec) {
		const envKey = "CLI_TEST_TRUE_DEFAULT_BOOL_ENV" // the env tag of CommandWithTrueDefaultBools.Env

		var (
			got        = let.VarOf[*CommandWithTrueDefaultBools](s, nil)
			falseInput = let.Var(s, func(t *testcase.T) string {
				return random.Pick(t.Random, "false", "False", "FALSE", "f", "F", "0")
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return CommandWithTrueDefaultBools{
				Callback: func(v CommandWithTrueDefaultBools, w cli.ResponseWriter, r *cli.Request) {
					got.Set(t, &v)
				},
			}
		})

		s.Before(func(t *testcase.T) {
			testcase.UnsetEnv(t, envKey)
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("the bool inputs the user didn't provide are true", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Flag, true)
			assert.Equal(t, got.Get(t).Env, true)
			assert.Equal(t, got.Get(t).Arg, true)
		})

		s.When("the user passes false with the flag", func(s *testcase.Spec) {
			request.Let(s, func(t *testcase.T) *cli.Request {
				r := request.Super(t)
				r.Args = append(r.Args, "-flag="+falseInput.Get(t))
				return r
			})

			s.Then("the flag is false instead of its true default", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Flag, false)
				assert.Equal(t, got.Get(t).Env, true)
				assert.Equal(t, got.Get(t).Arg, true)
			})
		})

		s.When("the environment variable is set to false", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, envKey, falseInput.Get(t))
			})

			s.Then("the env input is false instead of its true default", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Env, false)
				assert.Equal(t, got.Get(t).Flag, true)
				assert.Equal(t, got.Get(t).Arg, true)
			})
		})

		s.When("the user passes false as the argument", func(s *testcase.Spec) {
			request.Let(s, func(t *testcase.T) *cli.Request {
				r := request.Super(t)
				r.Args = append(r.Args, falseInput.Get(t))
				return r
			})

			s.Then("the arg is false instead of its true default", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Arg, false)
				assert.Equal(t, got.Get(t).Flag, true)
				assert.Equal(t, got.Get(t).Env, true)
			})
		})
	})

	s.Context("slice args with a default tag", func(s *testcase.Spec) {
		const ( // the env tags of CommandWithSliceArgDefaults
			rangeEnvKey    = "CLI_TEST_SLICE_ARG_RANGE_ENV"
			variadicEnvKey = "CLI_TEST_SLICE_ARG_VARIADIC_ENV"
		)

		var (
			got = let.VarOf[*CommandWithSliceArgDefaults](s, nil)
			cmd = let.Var(s, func(t *testcase.T) CommandWithSliceArgDefaults {
				return CommandWithSliceArgDefaults{
					Callback: func(v CommandWithSliceArgDefaults, w cli.ResponseWriter, r *cli.Request) {
						got.Set(t, &v)
					},
				}
			})
			userRange = let.Var(s, func(t *testcase.T) []string {
				return makeWords(t, 2)
			})
			userVariadic = let.Var(s, func(t *testcase.T) []string {
				return makeWords(t, t.Random.IntBetween(1, 5))
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return cmd.Get(t)
		})

		givenTheUserProvidesTheArgs := func(s *testcase.Spec) {
			request.Let(s, func(t *testcase.T) *cli.Request {
				r := request.Super(t)
				r.Args = append(r.Args, userRange.Get(t)...)
				r.Args = append(r.Args, userVariadic.Get(t)...)
				return r
			})
		}

		s.Before(func(t *testcase.T) {
			testcase.UnsetEnv(t, rangeEnvKey)
			testcase.UnsetEnv(t, variadicEnvKey)
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("default values are used for the args the user didn't provide", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Range, []string{"r1", "r2"})
			assert.Equal(t, got.Get(t).Variadic, []string{"v1", "v2", "v3"})
		})

		s.When("the user provides the args", func(s *testcase.Spec) {
			givenTheUserProvidesTheArgs(s)

			s.Then("the user input takes precedence over the default tag values", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Range, userRange.Get(t))
				assert.Equal(t, got.Get(t).Variadic, userVariadic.Get(t))
			})
		})

		s.When("the user provides only the range args", func(s *testcase.Spec) {
			request.Let(s, func(t *testcase.T) *cli.Request {
				r := request.Super(t)
				r.Args = append(r.Args, userRange.Get(t)...)
				return r
			})

			s.Then("the range arg gets the user input, and the variadic arg gets its default value", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Range, userRange.Get(t))
				assert.Equal(t, got.Get(t).Variadic, []string{"v1", "v2", "v3"})
			})
		})

		s.When("the environment variables of the args are set", func(s *testcase.Spec) {
			var (
				envRange = let.Var(s, func(t *testcase.T) []string {
					return makeWords(t, 2)
				})
				envVariadic = let.Var(s, func(t *testcase.T) []string {
					return makeWords(t, t.Random.IntBetween(1, 5))
				})
			)
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, rangeEnvKey, strings.Join(envRange.Get(t), ","))
				testcase.SetEnv(t, variadicEnvKey, strings.Join(envVariadic.Get(t), ","))
			})

			s.Then("the env values take precedence over the default tag values", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Range, envRange.Get(t))
				assert.Equal(t, got.Get(t).Variadic, envVariadic.Get(t))
			})

			s.And("the user provides the args", func(s *testcase.Spec) {
				givenTheUserProvidesTheArgs(s)

				s.Then("the user input takes precedence over the env values", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).Range, userRange.Get(t))
					assert.Equal(t, got.Get(t).Variadic, userVariadic.Get(t))
				})
			})
		})

		s.When("the registered handler has pre-set values", func(s *testcase.Spec) {
			var (
				preRange = let.Var(s, func(t *testcase.T) []string {
					return makeWords(t, 2)
				})
				preVariadic = let.Var(s, func(t *testcase.T) []string {
					return makeWords(t, t.Random.IntBetween(1, 5))
				})
			)
			cmd.Let(s, func(t *testcase.T) CommandWithSliceArgDefaults {
				c := cmd.Super(t)
				c.Range = preRange.Get(t)
				c.Variadic = preVariadic.Get(t)
				return c
			})

			s.Then("the pre-set values are kept instead of being overwritten by the default tag", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Range, preRange.Get(t))
				assert.Equal(t, got.Get(t).Variadic, preVariadic.Get(t))
			})

			s.And("the user provides the args", func(s *testcase.Spec) {
				givenTheUserProvidesTheArgs(s)

				s.Then("the user input takes precedence over the pre-set values", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).Range, userRange.Get(t))
					assert.Equal(t, got.Get(t).Variadic, userVariadic.Get(t))
				})
			})
		})

		s.When("the registered handler has only the variadic arg pre-set", func(s *testcase.Spec) {
			preVariadic := let.Var(s, func(t *testcase.T) []string {
				return makeWords(t, t.Random.IntBetween(1, 5))
			})
			cmd.Let(s, func(t *testcase.T) CommandWithSliceArgDefaults {
				c := cmd.Super(t)
				c.Variadic = preVariadic.Get(t)
				return c
			})

			s.Then("the pre-set value is kept, and the range arg gets its default value", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Variadic, preVariadic.Get(t))
				assert.Equal(t, got.Get(t).Range, []string{"r1", "r2"})
			})
		})
	})

	s.Context("required slice args", func(s *testcase.Spec) {
		const ( // the env tags of CommandWithRequiredSliceArgs
			rangeEnvKey    = "CLI_TEST_REQUIRED_SLICE_ARG_RANGE_ENV"
			variadicEnvKey = "CLI_TEST_REQUIRED_SLICE_ARG_VARIADIC_ENV"
		)

		var (
			got = let.VarOf[*CommandWithRequiredSliceArgs](s, nil)
			cmd = let.Var(s, func(t *testcase.T) CommandWithRequiredSliceArgs {
				return CommandWithRequiredSliceArgs{
					Callback: func(v CommandWithRequiredSliceArgs, w cli.ResponseWriter, r *cli.Request) {
						got.Set(t, &v)
					},
				}
			})
			userRange = let.Var(s, func(t *testcase.T) []string {
				return makeWords(t, 2)
			})
			userVariadic = let.Var(s, func(t *testcase.T) []string {
				return makeWords(t, t.Random.IntBetween(1, 5))
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return cmd.Get(t)
		})
		request.Let(s, func(t *testcase.T) *cli.Request {
			r := request.Super(t)
			r.Args = append(r.Args, userRange.Get(t)...)
			r.Args = append(r.Args, userVariadic.Get(t)...)
			return r
		})

		s.Before(func(t *testcase.T) {
			testcase.UnsetEnv(t, rangeEnvKey)
			testcase.UnsetEnv(t, variadicEnvKey)
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("the handler gets the user input", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Range, userRange.Get(t))
			assert.Equal(t, got.Get(t).Variadic, userVariadic.Get(t))
		})

		s.When("the user provides only the range args", func(s *testcase.Spec) {
			userVariadic.Let(s, func(t *testcase.T) []string {
				return nil
			})

			s.Then("it is rejected as a bad request without calling the handler", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "missing arguments for")
				assert.Contains(t, response.Get(t).Err.String(), "Variadic")
				assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
			})
		})

		s.When("the user doesn't provide any of the args", func(s *testcase.Spec) {
			userRange.Let(s, func(t *testcase.T) []string {
				return nil
			})
			userVariadic.Let(s, func(t *testcase.T) []string {
				return nil
			})

			s.Then("it is rejected as a bad request without calling the handler", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "missing arguments for")
				assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
			})

			s.And("the registered handler has pre-set values", func(s *testcase.Spec) {
				var (
					preRange = let.Var(s, func(t *testcase.T) []string {
						return makeWords(t, 2)
					})
					preVariadic = let.Var(s, func(t *testcase.T) []string {
						return makeWords(t, t.Random.IntBetween(1, 5))
					})
				)
				cmd.Let(s, func(t *testcase.T) CommandWithRequiredSliceArgs {
					c := cmd.Super(t)
					c.Range = preRange.Get(t)
					c.Variadic = preVariadic.Get(t)
					return c
				})

				s.Then("the pre-set values satisfy the requirement, and they are kept", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).Range, preRange.Get(t))
					assert.Equal(t, got.Get(t).Variadic, preVariadic.Get(t))
				})
			})

			s.And("the environment variables of the args are set", func(s *testcase.Spec) {
				var (
					envRange = let.Var(s, func(t *testcase.T) []string {
						return makeWords(t, 2)
					})
					envVariadic = let.Var(s, func(t *testcase.T) []string {
						return makeWords(t, t.Random.IntBetween(1, 5))
					})
				)
				s.Before(func(t *testcase.T) {
					testcase.SetEnv(t, rangeEnvKey, strings.Join(envRange.Get(t), ","))
					testcase.SetEnv(t, variadicEnvKey, strings.Join(envVariadic.Get(t), ","))
				})

				s.Then("the env values satisfy the requirement, and they are used", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).Range, envRange.Get(t))
					assert.Equal(t, got.Get(t).Variadic, envVariadic.Get(t))
				})
			})
		})
	})

	s.Context("slice inputs without a separator tag", func(s *testcase.Spec) {
		const envKey = "CLI_TEST_SLICE_WITHOUT_SEP_ENV" // the env tag of CommandWithSliceInputsWithoutSeparatorTag.Env

		var (
			got       = let.VarOf[*CommandWithSliceInputsWithoutSeparatorTag](s, nil)
			flagWords = let.Var(s, func(t *testcase.T) []string {
				return makeWords(t, t.Random.IntBetween(2, 5))
			})
			flagValue = let.Var(s, func(t *testcase.T) string {
				return strings.Join(flagWords.Get(t), ",")
			})
			envInts = let.Var(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(2, 5), func() int {
					return t.Random.IntBetween(0, 1000)
				})
			})
			envValue = let.Var(s, func(t *testcase.T) string {
				return strings.Join(slicekit.Map(envInts.Get(t), strconv.Itoa), ",")
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return CommandWithSliceInputsWithoutSeparatorTag{
				Callback: func(v CommandWithSliceInputsWithoutSeparatorTag, w cli.ResponseWriter, r *cli.Request) {
					got.Set(t, &v)
				},
			}
		})
		request.Let(s, func(t *testcase.T) *cli.Request {
			r := request.Super(t)
			r.Args = append(r.Args, "-flag", flagValue.Get(t))
			return r
		})

		s.Before(func(t *testcase.T) {
			testcase.SetEnv(t, envKey, envValue.Get(t))
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("the flag value is split on commas", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Flag, flagWords.Get(t))
		})

		s.Then("the env value is split on commas", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Env, envInts.Get(t))
		})

		s.Then("the default tag value is split on commas", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Default, []string{"foo", "bar", "baz"})
		})

		s.When("the flag is repeated", func(s *testcase.Spec) {
			moreWords := let.Var(s, func(t *testcase.T) []string {
				return makeWords(t, t.Random.IntBetween(1, 3))
			})
			request.Let(s, func(t *testcase.T) *cli.Request {
				r := request.Super(t)
				r.Args = append(r.Args, "-flag", strings.Join(moreWords.Get(t), ","))
				return r
			})

			s.Then("each flag value is split on commas, and the values are combined", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Flag, append(slicekit.Clone(flagWords.Get(t)), moreWords.Get(t)...))
			})
		})

		s.When("the env value is a single number, which is also valid JSON", func(s *testcase.Spec) {
			envInts.Let(s, func(t *testcase.T) []int {
				return []int{t.Random.IntBetween(0, 1000)}
			})

			s.Then("it is parsed as a list with a single element, since only JSON arrays are parsed as JSON", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Env, envInts.Get(t))
			})
		})

		s.When("the values are JSON arrays", func(s *testcase.Spec) {
			flagWords.Let(s, func(t *testcase.T) []string {
				return random.Slice(t.Random.IntBetween(1, 3), func() string {
					return strings.Join(makeWords(t, 2), ",") // a comma inside of a JSON array element
				})
			})
			flagValue.Let(s, func(t *testcase.T) string {
				data, err := json.Marshal(flagWords.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
			envValue.Let(s, func(t *testcase.T) string {
				data, err := json.Marshal(envInts.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})

			s.Then("they are parsed as JSON, instead of being split on commas", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Flag, flagWords.Get(t))
				assert.Equal(t, got.Get(t).Env, envInts.Get(t))
			})
		})
	})

	s.Context("slice inputs with a separator tag", func(s *testcase.Spec) {
		const (
			envKey    = "CLI_TEST_SLICE_WITH_SEP_ENV" // the env tag of CommandWithSliceInputsWithSeparatorTag.Env
			separator = ";"                           // the sep tag of the CommandWithSliceInputsWithSeparatorTag fields
		)

		var (
			got       = let.VarOf[*CommandWithSliceInputsWithSeparatorTag](s, nil)
			flagWords = let.Var(s, func(t *testcase.T) []string {
				return makeWords(t, t.Random.IntBetween(2, 5))
			})
			flagValue = let.Var(s, func(t *testcase.T) string {
				return strings.Join(flagWords.Get(t), separator)
			})
			envInts = let.Var(s, func(t *testcase.T) []int {
				return random.Slice(t.Random.IntBetween(2, 5), func() int {
					return t.Random.IntBetween(0, 1000)
				})
			})
			envValue = let.Var(s, func(t *testcase.T) string {
				return strings.Join(slicekit.Map(envInts.Get(t), strconv.Itoa), separator)
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return CommandWithSliceInputsWithSeparatorTag{
				Callback: func(v CommandWithSliceInputsWithSeparatorTag, w cli.ResponseWriter, r *cli.Request) {
					got.Set(t, &v)
				},
			}
		})
		request.Let(s, func(t *testcase.T) *cli.Request {
			r := request.Super(t)
			r.Args = append(r.Args, "-flag", flagValue.Get(t))
			return r
		})

		s.Before(func(t *testcase.T) {
			testcase.SetEnv(t, envKey, envValue.Get(t))
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("the values are split on the separator", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Flag, flagWords.Get(t))
			assert.Equal(t, got.Get(t).Env, envInts.Get(t))
		})

		s.When("the env value is a single number, which is also valid JSON", func(s *testcase.Spec) {
			envInts.Let(s, func(t *testcase.T) []int {
				return []int{t.Random.IntBetween(0, 1000)}
			})

			s.Then("it is parsed as a list with a single element, since only JSON arrays are parsed as JSON", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Env, envInts.Get(t))
			})
		})

		s.When("the values are JSON arrays", func(s *testcase.Spec) {
			flagWords.Let(s, func(t *testcase.T) []string {
				return random.Slice(t.Random.IntBetween(1, 3), func() string {
					return strings.Join(makeWords(t, 2), separator) // the separator inside of a JSON array element
				})
			})
			flagValue.Let(s, func(t *testcase.T) string {
				data, err := json.Marshal(flagWords.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
			envValue.Let(s, func(t *testcase.T) string {
				data, err := json.Marshal(envInts.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})

			s.Then("they are parsed as JSON, the same way as the env package does", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Flag, flagWords.Get(t))
				assert.Equal(t, got.Get(t).Env, envInts.Get(t))
			})
		})
	})

	s.Context("time.Time field", func(s *testcase.Spec) {
		const layout = "2006-01-02" // the layout tag of CommandWithTimeFlags.Date and CommandWithTimeFlags.Since

		var (
			got  = let.VarOf[*CommandWithTimeFlags](s, nil)
			date = let.Var(s, func(t *testcase.T) time.Time {
				d, err := time.Parse(layout, t.Random.Time().Format(layout))
				assert.Must(t).NoError(err)
				return d
			})
			args = let.Var(s, func(t *testcase.T) []string {
				return []string{"-date", date.Get(t).Format(layout)}
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return CommandWithTimeFlags{
				Callback: func(v CommandWithTimeFlags, w cli.ResponseWriter, r *cli.Request) {
					got.Set(t, &v)
				},
			}
		})
		request.Let(s, func(t *testcase.T) *cli.Request {
			r := request.Super(t)
			r.Args = append(r.Args, args.Get(t)...)
			return r
		})

		s.Before(func(t *testcase.T) {
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("the value is parsed with the layout from the layout tag", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.True(t, got.Get(t).Date.Equal(date.Get(t)),
				assert.MessageF("expected %s, got %s", date.Get(t), got.Get(t).Date))
		})

		s.Then("the default tag value is parsed with the layout from the layout tag", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			exp := time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC)
			assert.True(t, got.Get(t).Since.Equal(exp),
				assert.MessageF("expected %s, got %s", exp, got.Get(t).Since))
		})

		s.When("the field has no layout tag", func(s *testcase.Spec) {
			at := let.Time(s)

			args.Let(s, func(t *testcase.T) []string {
				return []string{"-at", at.Get(t).Format(time.RFC3339)}
			})

			s.Then("the value is parsed as RFC3339", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.True(t, got.Get(t).At.Equal(at.Get(t)),
					assert.MessageF("expected %s, got %s", at.Get(t), got.Get(t).At))
			})
		})

		s.When("the value doesn't match the layout", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-date", date.Get(t).Format("02/01/2006")}
			})

			s.Then("it is rejected as a bad request without calling the handler", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "Invalid input")
				assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
			})
		})

		s.When("help is requested", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-help"}
			})

			s.Then("the expected layout of each time.Time flag is documented", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				out := response.Get(t).Out.String()
				assert.MatchRegexp(t, out, `-date=\S+.*`+regexp.QuoteMeta(layout))
				assert.MatchRegexp(t, out, `-at=\S+.*`+regexp.QuoteMeta(time.RFC3339))
			})
		})
	})

	s.Context(`fields tagged as opaque`, func(s *testcase.Spec) {
		const (
			tokenEnvKey  = "CLI_TEST_FOREIGN_TOKEN_ENV"  // the env tag of ForeignConfig.Token
			configEnvKey = "CLI_TEST_FOREIGN_CONFIG_ENV" // the env tag of CommandWithOpaqueFields.Config
		)

		var (
			got        = let.VarOf[*CommandWithOpaqueFields](s, nil)
			name       = let.StringNC(s, 8, random.CharsetAlpha())
			dependency = let.Var(s, func(t *testcase.T) *ForeignConfig {
				// Name is left empty, so its default tag value would show if the cli package reached it.
				return &ForeignConfig{Token: t.Random.StringNC(8, random.CharsetAlpha())}
			})
			config = let.Var(s, func(t *testcase.T) ForeignConfig {
				return ForeignConfig{
					Name:  t.Random.StringNC(8, random.CharsetAlpha()),
					Token: t.Random.StringNC(8, random.CharsetAlpha()),
					Paths: makeWords(t, t.Random.IntBetween(1, 3)),
				}
			})
			configJSON = let.Var(s, func(t *testcase.T) string {
				data, err := json.Marshal(config.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
			args = let.Var(s, func(t *testcase.T) []string {
				return []string{"-name", name.Get(t)}
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return CommandWithOpaqueFields{
				Callback: func(v CommandWithOpaqueFields, w cli.ResponseWriter, r *cli.Request) {
					got.Set(t, &v)
				},
				Pointer: dependency.Get(t),
			}
		})
		request.Let(s, func(t *testcase.T) *cli.Request {
			r := request.Super(t)
			r.Args = append(r.Args, args.Get(t)...)
			return r
		})

		s.Before(func(t *testcase.T) {
			// ForeignConfig.Token is required, so it would fail the command if it were an input.
			testcase.UnsetEnv(t, tokenEnvKey)
			testcase.UnsetEnv(t, configEnvKey)
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("the handler's own inputs are configured, even when an opaque field's type uses the same input names", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Name, name.Get(t))
		})

		s.Then("the fields of an opaque field's type are not inputs, so their default tags are not applied, and their required tags are not enforced", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).ForeignConfig, ForeignConfig{})
			assert.Equal(t, got.Get(t).Field, ForeignConfig{})
			assert.Equal(t, got.Get(t).Config, ForeignConfig{})
		})

		s.Then("the norecurse alias works the same as the opaque tag", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Alias, ForeignConfig{})
		})

		s.Then(`a field tagged with opaque:"false" is looked into as usual`, func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Settings.Level, "info")
		})

		s.Then("a dependency behind an opaque pointer field is left untouched", func(t *testcase.T) {
			exp := *dependency.Get(t)

			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.True(t, got.Get(t).Pointer == dependency.Get(t))
			assert.Equal(t, *dependency.Get(t), exp)
		})

		s.When("the env var of an opaque field's type is set", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, tokenEnvKey, t.Random.StringNC(8, random.CharsetAlpha()))
			})

			s.Then("it is not read", func(t *testcase.T) {
				exp := *dependency.Get(t)

				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Empty(t, got.Get(t).ForeignConfig.Token)
				assert.Empty(t, got.Get(t).Field.Token)
				assert.Empty(t, got.Get(t).Alias.Token)
				assert.Empty(t, got.Get(t).Config.Token)
				assert.Equal(t, *dependency.Get(t), exp)
			})
		})

		s.When("a flag of an opaque field's type is given", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-token", t.Random.StringNC(8, random.CharsetAlpha())}
			})

			s.Then("it is rejected as an undefined flag without calling the handler", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "-token")
				assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
			})
		})

		s.When("the opaque field is an input itself, and its flag gets a JSON value", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-config", configJSON.Get(t)}
			})

			s.Then("the JSON value is parsed into the field", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Config, config.Get(t))
			})

			s.And("the JSON value doesn't match the field's type", func(s *testcase.Spec) {
				configJSON.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf(`{"Name":%d}`, t.Random.Int())
				})

				s.Then("it is rejected as a bad request without calling the handler", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					assert.Contains(t, response.Get(t).Err.String(), "Invalid input")
					assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
				})
			})
		})

		s.When("the opaque field is an input itself, and its env var holds a JSON value", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, configEnvKey, configJSON.Get(t))
			})

			s.Then("the JSON value is parsed into the field", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Config, config.Get(t))
			})
		})

		s.When("help is requested", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-help"}
			})

			s.Then("only the handler's own inputs are documented", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				out := response.Get(t).Out.String()
				assert.Contains(t, out, "-name")
				assert.Contains(t, out, "-config")
				assert.Contains(t, out, "-level")
				assert.Contains(t, out, configEnvKey)
				assert.NotContains(t, out, "-token")
				assert.NotContains(t, out, tokenEnvKey)
				assert.NotContains(t, out, "foreign-default")
				assert.NotContains(t, out, "Paths")
			})
		})

		s.When("the opaque tag has an invalid value", func(s *testcase.Spec) {
			handler.Let(s, func(t *testcase.T) cli.Handler {
				return CommandWithInvalidOpaqueTagValue{}
			})

			s.Then("it panics with the field, the tag and the invalid value, so a mistyped tag can't go unnoticed", func(t *testcase.T) {
				msg := fmt.Sprint(assert.Panic(t, func() { act(t) }))
				assert.Contains(t, msg, "Settings")
				assert.Contains(t, msg, "opaque")
				assert.Contains(t, msg, "maybe")
			})
		})
	})

	s.Context(`struct fields which are inputs themselves`, func(s *testcase.Spec) {
		const (
			retryEnvKey  = "CLI_TEST_RETRY_ENV"   // the env tag of CommandWithStructInputs.Retry
			dbEnvKey     = "CLI_TEST_DB_ENV"      // the env tag of CommandWithStructInputs.DB
			dbHostEnvKey = "CLI_TEST_DB_HOST_ENV" // the env tag of DBConfig.Host
			dbPortEnvKey = "CLI_TEST_DB_PORT_ENV" // the env tag of DBConfig.Port

			authEnvKey      = "CLI_TEST_AUTH_ENV"       // the env tag of CommandWithStructInputs.Auth
			authTokenEnvKey = "CLI_TEST_AUTH_TOKEN_ENV" // the env tag of AuthConfig.Token
		)

		var (
			got  = let.VarOf[*CommandWithStructInputs](s, nil)
			args = let.Var(s, func(t *testcase.T) []string {
				return nil
			})
			retry = let.Var(s, func(t *testcase.T) RetryPolicy {
				return RetryPolicy{
					Max:     t.Random.IntBetween(1, 10),
					Backoff: time.Duration(t.Random.IntBetween(1, 60)) * time.Second,
				}
			})
			retryJSON = let.Var(s, func(t *testcase.T) string {
				data, err := json.Marshal(retry.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
			db = let.Var(s, func(t *testcase.T) DBConfig {
				return DBConfig{
					Host: t.Random.StringNC(8, random.CharsetAlpha()),
					Port: t.Random.IntBetween(2000, 2999),
				}
			})
			dbJSON = let.Var(s, func(t *testcase.T) string {
				data, err := json.Marshal(db.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
			// the inner values use a port range that differs from db's, so they can't be mistaken for each other.
			innerHost = let.StringNC(s, 8, random.CharsetAlpha())
			innerPort = let.IntB(s, 1000, 1999)
			// innerToken is set as the env var of the required inner field, so the scenarios not about it stay valid.
			innerToken = let.StringNC(s, 8, random.CharsetAlpha())
			auth       = let.Var(s, func(t *testcase.T) AuthConfig {
				return AuthConfig{Token: t.Random.StringNC(8, random.CharsetAlpha())}
			})
			authJSON = let.Var(s, func(t *testcase.T) string {
				data, err := json.Marshal(auth.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return CommandWithStructInputs{
				Callback: func(v CommandWithStructInputs, w cli.ResponseWriter, r *cli.Request) {
					got.Set(t, &v)
				},
			}
		})
		request.Let(s, func(t *testcase.T) *cli.Request {
			r := request.Super(t)
			r.Args = append(r.Args, args.Get(t)...)
			return r
		})

		s.Before(func(t *testcase.T) {
			testcase.UnsetEnv(t, retryEnvKey)
			testcase.UnsetEnv(t, dbEnvKey)
			testcase.UnsetEnv(t, dbHostEnvKey)
			testcase.UnsetEnv(t, dbPortEnvKey)
			testcase.UnsetEnv(t, authEnvKey)
			testcase.SetEnv(t, authTokenEnvKey, innerToken.Get(t))
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("the inner fields are configured from their own tags, like their defaults", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).DB, DBConfig{Host: "localhost", Port: 5432})
			assert.Equal(t, got.Get(t).Retry, RetryPolicy{})
		})

		s.Then("a required inner field is configured from its own input", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Auth, AuthConfig{Token: innerToken.Get(t)})
		})

		s.When("the flag of a struct field without cli tags of its own gets a JSON value", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-retry", retryJSON.Get(t)}
			})

			s.Then("the JSON value is parsed into the field", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Retry, retry.Get(t))
			})

			s.And("the JSON value doesn't match the field's type", func(s *testcase.Spec) {
				retryJSON.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf(`{"Max":%q}`, t.Random.StringNC(8, random.CharsetAlpha()))
				})

				s.Then("it is rejected as a bad request without calling the handler", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					assert.Contains(t, response.Get(t).Err.String(), "Invalid input")
					assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
				})
			})
		})

		s.When("the env var of a struct field without cli tags of its own holds a JSON value", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, retryEnvKey, retryJSON.Get(t))
			})

			s.Then("the JSON value is parsed into the field", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Retry, retry.Get(t))
			})
		})

		s.When("the flags of the inner fields are given", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-db-host", innerHost.Get(t), "-db-port", strconv.Itoa(innerPort.Get(t))}
			})

			s.Then("the inner fields are configured from them", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).DB, DBConfig{Host: innerHost.Get(t), Port: innerPort.Get(t)})
			})
		})

		s.When("the flag of a struct field with cli tags of its own gets a JSON value", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-db", dbJSON.Get(t)}
			})

			s.Then("the JSON value is parsed onto the inner fields, overwriting their defaults", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).DB, db.Get(t))
			})

			s.And("the flags of the inner fields are given too, even after it", func(s *testcase.Spec) {
				args.Let(s, func(t *testcase.T) []string {
					return append(args.Super(t), "-db-host", innerHost.Get(t), "-db-port", strconv.Itoa(innerPort.Get(t)))
				})

				s.Then("the inner fields are configured first, then the JSON value overwrites them", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).DB, db.Get(t))
				})
			})

			s.And("the JSON value has only some of the fields", func(s *testcase.Spec) {
				dbJSON.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf(`{"Host":%q}`, db.Get(t).Host)
				})

				s.Then("the JSON value is merged onto the inner fields, so the absent ones keep their configured value", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).DB, DBConfig{Host: db.Get(t).Host, Port: 5432})
				})
			})

			s.And("the JSON value doesn't match the field's type", func(s *testcase.Spec) {
				dbJSON.Let(s, func(t *testcase.T) string {
					return fmt.Sprintf(`{"Port":%q}`, t.Random.StringNC(8, random.CharsetAlpha()))
				})

				s.Then("it is rejected as a bad request without calling the handler", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					assert.Contains(t, response.Get(t).Err.String(), "Invalid input")
					assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
				})
			})
		})

		s.When("the env var of a struct field with cli tags of its own holds a JSON value", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, dbEnvKey, dbJSON.Get(t))
			})

			s.Then("the JSON value is parsed onto the inner fields, overwriting their defaults", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).DB, db.Get(t))
			})

			s.And("the env vars of the inner fields are set too", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					testcase.SetEnv(t, dbHostEnvKey, innerHost.Get(t))
					testcase.SetEnv(t, dbPortEnvKey, strconv.Itoa(innerPort.Get(t)))
				})

				s.Then("the inner fields are configured first, then the JSON value overwrites them", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).DB, db.Get(t))
				})
			})
		})

		// required is checked once the whole configuring is complete,
		// so a required inner field can be satisfied by the JSON value of its outer struct field too.
		s.When("a required inner field gets no input", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.UnsetEnv(t, authTokenEnvKey)
			})

			s.Then("it is rejected as a bad request that names the input, without calling the handler", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "auth-token")
				assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
			})

			s.And("the flag of the outer struct field gets a JSON value with it", func(s *testcase.Spec) {
				args.Let(s, func(t *testcase.T) []string {
					return []string{"-auth", authJSON.Get(t)}
				})

				s.Then("required is satisfied by the JSON value", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).Auth, auth.Get(t))
				})
			})

			s.And("the env var of the outer struct field holds a JSON value with it", func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					testcase.SetEnv(t, authEnvKey, authJSON.Get(t))
				})

				s.Then("required is satisfied by the JSON value", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
					assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
					assert.Equal(t, got.Get(t).Auth, auth.Get(t))
				})
			})

			s.And("the outer struct field gets a JSON value without it", func(s *testcase.Spec) {
				args.Let(s, func(t *testcase.T) []string {
					return []string{"-auth", `{}`}
				})

				s.Then("it is still rejected as a bad request that names the input, without calling the handler", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					assert.Contains(t, response.Get(t).Err.String(), "auth-token")
					assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
				})
			})
		})

		s.When("the env var of a required inner field is set to an empty value", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, authTokenEnvKey, "")
			})

			s.Then("required is satisfied, since the input is provided, even though the configured value is empty", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Auth, AuthConfig{})
			})
		})

		s.When("the flag of a required inner field gets an empty value", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.UnsetEnv(t, authTokenEnvKey)
			})
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-auth-token", ""}
			})

			s.Then("required is satisfied, since the input is provided, even though the configured value is empty", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Auth, AuthConfig{})
			})
		})
	})

	s.Context(`struct fields with a JSON value in their default tag`, func(s *testcase.Spec) {
		const (
			replicaEnvKey      = "CLI_TEST_REPLICA_ENV"       // the env tag of CommandWithStructInputDefault.Replica
			replicaHostEnvKey  = "CLI_TEST_REPLICA_HOST_ENV"  // the env tag of ReplicaConfig.Host
			replicaPortEnvKey  = "CLI_TEST_REPLICA_PORT_ENV"  // the env tag of ReplicaConfig.Port
			replicaUserEnvKey  = "CLI_TEST_REPLICA_USER_ENV"  // the env tag of ReplicaConfig.User
			replicaTokenEnvKey = "CLI_TEST_REPLICA_TOKEN_ENV" // the env tag of ReplicaConfig.Token
		)
		// the values in the default tag of CommandWithStructInputDefault.Replica
		const (
			defaultHost  = "replica.internal"
			defaultToken = "from-default"
		)

		var (
			got  = let.VarOf[*CommandWithStructInputDefault](s, nil)
			args = let.Var(s, func(t *testcase.T) []string {
				return nil
			})
			// innerUser is set as the env var of the required inner field that the default JSON lacks,
			// so the scenarios not about it stay valid.
			innerUser = let.StringNC(s, 8, random.CharsetAlpha())
		)
		handler.Let(s, func(t *testcase.T) cli.Handler {
			return CommandWithStructInputDefault{
				Callback: func(v CommandWithStructInputDefault, w cli.ResponseWriter, r *cli.Request) {
					got.Set(t, &v)
				},
			}
		})
		request.Let(s, func(t *testcase.T) *cli.Request {
			r := request.Super(t)
			r.Args = append(r.Args, args.Get(t)...)
			return r
		})

		s.Before(func(t *testcase.T) {
			testcase.UnsetEnv(t, replicaEnvKey)
			testcase.UnsetEnv(t, replicaHostEnvKey)
			testcase.UnsetEnv(t, replicaPortEnvKey)
			testcase.UnsetEnv(t, replicaTokenEnvKey)
			testcase.SetEnv(t, replicaUserEnvKey, innerUser.Get(t))
			debugRecorderResult(t, response.Get(t))
		})

		s.Then("the default JSON is merged onto the inner fields, overwriting their defaults, while the absent ones keep theirs", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Replica.Host, defaultHost)
			assert.Equal(t, got.Get(t).Replica.Port, 5432)
		})

		s.Then("the default JSON satisfies a required inner field that has no input", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
			assert.Equal(t, got.Get(t).Replica.Token, defaultToken)
		})

		s.When("the flags of the inner fields are given", func(s *testcase.Spec) {
			var (
				innerHost  = let.StringNC(s, 8, random.CharsetAlpha())
				innerPort  = let.IntB(s, 1000, 1999)
				innerToken = let.StringNC(s, 8, random.CharsetAlpha())
			)
			args.Let(s, func(t *testcase.T) []string {
				return []string{
					"-replica-host", innerHost.Get(t),
					"-replica-port", strconv.Itoa(innerPort.Get(t)),
					"-replica-token", innerToken.Get(t),
				}
			})

			s.Then("they win over the default JSON, since a default never overwrites an input", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Replica, ReplicaConfig{
					Host:  innerHost.Get(t),
					Port:  innerPort.Get(t),
					User:  innerUser.Get(t),
					Token: innerToken.Get(t),
				})
			})
		})

		s.When("the flag of the struct field gets a JSON value", func(s *testcase.Spec) {
			port := let.IntB(s, 2000, 2999)
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-replica", fmt.Sprintf(`{"Port":%d}`, port.Get(t))}
			})

			s.Then("it is merged onto the value the field has without it, including its default JSON", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.NotNil(t, got.Get(t), assert.Message("handler was not called"))
				assert.Equal(t, got.Get(t).Replica, ReplicaConfig{
					Host:  defaultHost,
					Port:  port.Get(t),
					User:  innerUser.Get(t),
					Token: defaultToken,
				})
			})
		})

		s.When("a required inner field that the default JSON lacks gets no input", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.UnsetEnv(t, replicaUserEnvKey)
			})

			s.Then("it is rejected as a bad request that names the input, without calling the handler", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "replica-user")
				assert.Nil(t, got.Get(t), assert.Message("handler should not be called"))
			})
		})
	})

	s.Context("framework generated text", func(s *testcase.Spec) {
		// Help, usage and input error reports are written by the framework, not by the handler.
		// They should look the same on every OS, so tests can assert them without OS specific branches.
		var args = let.Var(s, func(t *testcase.T) []string {
			return []string{"-help"}
		})
		handler.Let(s, func(t *testcase.T) cli.Handler {
			var mux cli.ServeMux
			mux.Handle("cmd", CommandWithFlagWithEnum{})
			return &mux
		})
		request.Let(s, func(t *testcase.T) *cli.Request {
			r := request.Super(t)
			r.Args = append(r.Args, args.Get(t)...)
			return r
		})

		s.Before(func(t *testcase.T) {
			in := slicekit.Clone(args.Get(t))
			t.OnFail(func() {
				t.Logf("args: %q", in)
				t.Logf("output: %q", response.Get(t).CombinedOutput())
			})
		})

		thenItIsOSIndependent := func(s *testcase.Spec) {
			s.Then("line endings are LF on every OS", func(t *testcase.T) {
				act(t)

				out := string(response.Get(t).CombinedOutput())
				assert.Contains(t, out, "\n")
				assert.NotContains(t, out, "\r")
			})

			s.Then("the usage names the program the same way on every OS", func(t *testcase.T) {
				act(t)

				out := string(response.Get(t).CombinedOutput())
				usage := regexp.MustCompile(`(?m)^Usage: (\S+)`).FindStringSubmatch(out)
				assert.Equal(t, len(usage), 2, assert.Message("usage line is missing"))
				assert.False(t, strings.HasSuffix(strings.ToLower(usage[1]), ".exe"),
					assert.MessageF("the program name has an OS specific extension: %q", usage[1]))
			})
		}

		thenItIsOSIndependent(s)

		s.When("the help of a command is requested", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"cmd", "-help"}
			})

			thenItIsOSIndependent(s)
		})

		s.When("the input is invalid", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"cmd", "-flag", "invalid"}
			})

			s.Then("the error report lists the accepted values", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "accepted values")
			})

			thenItIsOSIndependent(s)
		})

		s.When("the command is unknown", func(s *testcase.Spec) {
			unknown := let.StringNC(s, 8, random.CharsetAlpha())

			args.Let(s, func(t *testcase.T) []string {
				return []string{unknown.Get(t)}
			})

			thenItIsOSIndependent(s)
		})

		s.When("the executable has a Windows .exe extension", func(s *testcase.Spec) {
			var (
				name = let.StringNC(s, 8, random.CharsetAlpha())
				ext  = let.OneOf(s, ".exe", ".EXE", ".Exe")
			)
			s.Before(func(t *testcase.T) {
				osint.StubExecutable(t, filepath.Join("bin", name.Get(t)+ext.Get(t)))
			})

			s.Then("the usage names the program without the extension", func(t *testcase.T) {
				act(t)

				out := string(response.Get(t).CombinedOutput())
				usage := regexp.MustCompile(`(?m)^Usage: (\S+)`).FindStringSubmatch(out)
				assert.Equal(t, len(usage), 2, assert.Message("usage line is missing"))
				assert.Equal(t, usage[1], name.Get(t))
			})
		})

		s.When("the help of a command is requested with a known executable name", func(s *testcase.Spec) {
			name := let.StringNC(s, 8, random.CharsetAlpha())

			args.Let(s, func(t *testcase.T) []string {
				return []string{"cmd", "-help"}
			})
			s.Before(func(t *testcase.T) {
				osint.StubExecutable(t, filepath.Join("bin", name.Get(t)))
			})

			s.Then("the help text is the same on every OS, so it can be compared exactly", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.Equal(t, response.Get(t).Out.String(), ""+
					"Usage: "+name.Get(t)+" cmd [OPTION]...\n"+
					"\n"+
					"Options:\n"+
					"  -flag=[string]\n"+
					"\n"+
					"\n")
				assert.Empty(t, response.Get(t).Err.String())
			})
		})

		s.When("the command has args at a single position, a range and an open range", func(s *testcase.Spec) {
			handler.Let(s, func(t *testcase.T) cli.Handler {
				var mux cli.ServeMux
				mux.Handle("cmd", CommandWithArgPositions{})
				return &mux
			})
			args.Let(s, func(t *testcase.T) []string {
				return []string{"cmd", "-help"}
			})
			s.Before(func(t *testcase.T) {
				testcase.UnsetEnv(t, "CLI_TEST_ARG_POSITION_SINGLE_ENV")
				testcase.UnsetEnv(t, "CLI_TEST_ARG_POSITION_RANGE_ENV")
				testcase.UnsetEnv(t, "CLI_TEST_ARG_POSITION_VARIADIC_ENV")
			})

			s.Then("the help names the arg position that substitutes each env", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				out := response.Get(t).Out.String()
				assert.Contains(t, out, "Substitute args[0]\n")
				assert.Contains(t, out, "Substitute args[1:3]\n")
				assert.Contains(t, out, "Substitute args[3:]\n")
			})

			s.And("the single position arg is missing", func(s *testcase.Spec) {
				args.Let(s, func(t *testcase.T) []string {
					return []string{"cmd"}
				})

				s.Then("the error report names the arg by its single position", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					assert.Contains(t, response.Get(t).Err.String(), "Invalid input:\n\targ[0]\n")
				})
			})

			s.And("the range arg is missing", func(s *testcase.Spec) {
				args.Let(s, func(t *testcase.T) []string {
					return []string{"cmd", t.Random.StringNC(8, random.CharsetAlpha())}
				})

				s.Then("the error report names the arg by its range", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					assert.Contains(t, response.Get(t).Err.String(), "Invalid input:\n\targ[1:3]\n")
				})
			})

			s.And("the open range arg is missing", func(s *testcase.Spec) {
				args.Let(s, func(t *testcase.T) []string {
					return []string{"cmd",
						t.Random.StringNC(8, random.CharsetAlpha()),
						t.Random.StringNC(8, random.CharsetAlpha()),
						t.Random.StringNC(8, random.CharsetAlpha()),
					}
				})

				s.Then("the error report names the arg by its open range", func(t *testcase.T) {
					act(t)

					assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
					assert.Contains(t, response.Get(t).Err.String(), "Invalid input:\n\targ[3:]\n")
				})
			})
		})
	})
}

type CommandWithDefaultTags struct {
	Callback[CommandWithDefaultTags]

	// Dependency is not an input, it has no input tags.
	Dependency string

	Flag string `flag:"flag" default:"flag-default"`
	Bool bool   `flag:"bool" default:"true"`
	Env  string `env:"CLI_TEST_DEFAULT_TAG_ENV" default:"env-default"`
	Arg  string `arg:"0" default:"arg-default"`
}

func (cmd CommandWithDefaultTags) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithTrueDefaultBools struct {
	Callback[CommandWithTrueDefaultBools]

	Flag bool `flag:"flag" default:"true"`
	Env  bool `env:"CLI_TEST_TRUE_DEFAULT_BOOL_ENV" default:"true"`
	Arg  bool `arg:"0" default:"true"`
}

func (cmd CommandWithTrueDefaultBools) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithSliceArgDefaults struct {
	Callback[CommandWithSliceArgDefaults]

	Range    []string `arg:"0:2" env:"CLI_TEST_SLICE_ARG_RANGE_ENV" default:"r1,r2"`
	Variadic []string `arg:"2:" env:"CLI_TEST_SLICE_ARG_VARIADIC_ENV" default:"v1,v2,v3"`
}

func (cmd CommandWithSliceArgDefaults) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithRequiredSliceArgs struct {
	Callback[CommandWithRequiredSliceArgs]

	Range    []string `arg:"0:2" env:"CLI_TEST_REQUIRED_SLICE_ARG_RANGE_ENV"`
	Variadic []string `arg:"2:" env:"CLI_TEST_REQUIRED_SLICE_ARG_VARIADIC_ENV"`
}

func (cmd CommandWithRequiredSliceArgs) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithArgPositions struct {
	Callback[CommandWithArgPositions]

	Single   string   `arg:"0" env:"CLI_TEST_ARG_POSITION_SINGLE_ENV"`
	Range    []string `arg:"1:3" env:"CLI_TEST_ARG_POSITION_RANGE_ENV"`
	Variadic []string `arg:"3:" env:"CLI_TEST_ARG_POSITION_VARIADIC_ENV"`
}

func (cmd CommandWithArgPositions) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

// CommandWithFlagCollision has two fields of the same config type, so their flags have the same names.
type CommandWithFlagCollision struct {
	Callback[CommandWithFlagCollision]

	Primary FlagCollisionConfig
	Replica FlagCollisionConfig
}

type FlagCollisionConfig struct {
	Host string `flag:"host"`
}

func (cmd CommandWithFlagCollision) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

// CommandWithArgPositionGap has no arg at index 1.
type CommandWithArgPositionGap struct {
	Callback[CommandWithArgPositionGap]

	First string `arg:"0"`
	Third string `arg:"2"`
}

func (cmd CommandWithArgPositionGap) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithSliceInputsWithoutSeparatorTag struct {
	Callback[CommandWithSliceInputsWithoutSeparatorTag]

	Flag    []string `flag:"flag"`
	Env     []int    `env:"CLI_TEST_SLICE_WITHOUT_SEP_ENV" opt:"T"`
	Default []string `flag:"default" default:"foo,bar,baz"`
}

func (cmd CommandWithSliceInputsWithoutSeparatorTag) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithSliceInputsWithSeparatorTag struct {
	Callback[CommandWithSliceInputsWithSeparatorTag]

	Flag []string `flag:"flag" sep:";"`
	Env  []int    `env:"CLI_TEST_SLICE_WITH_SEP_ENV" sep:";" opt:"T"`
}

func (cmd CommandWithSliceInputsWithSeparatorTag) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

// makeWords makes n random words, which never contain a comma, and never start with a dash,
// so they can be used as command line args, and as comma separated list elements.
func makeWords(t *testcase.T, n int) []string {
	return random.Slice(n, func() string {
		return t.Random.StringNC(8, random.CharsetAlpha())
	})
}

type CommandWithTimeFlags struct {
	Callback[CommandWithTimeFlags]

	Date  time.Time `flag:"date" layout:"2006-01-02"`
	Since time.Time `flag:"since" layout:"2006-01-02" default:"2000-01-02"`
	At    time.Time `flag:"at"`
}

func (cmd CommandWithTimeFlags) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

// ForeignConfig mimics a type which was designed with different principles than the cli package,
// such as the command struct of another CLI framework, or the config type of a third-party library.
// Its tags share their names with the cli tags, but they don't follow the cli conventions.
type ForeignConfig struct {
	Name  string   `flag:"name" default:"foreign-default"` // the same flag name as CommandWithOpaqueFields.Name
	Token string   `flag:"token" env:"CLI_TEST_FOREIGN_TOKEN_ENV" required:"true"`
	Paths []string `arg:""` // a positional arg marker in kong, but not a valid arg index for the cli package
}

// CompatibleSettings follows the cli conventions, so its tags are inputs when the cli package looks into it.
type CompatibleSettings struct {
	Level string `flag:"level" default:"info"`
}

type CommandWithOpaqueFields struct {
	Callback[CommandWithOpaqueFields]

	Name string `flag:"name"`

	ForeignConfig `opaque:""`
	Field         ForeignConfig  `opaque:"true"`
	Alias         ForeignConfig  `norecurse:""`
	Pointer       *ForeignConfig `opaque:""`

	// Config is an input itself, which takes a JSON value, but the tags of its type are not inputs.
	Config ForeignConfig `flag:"config" env:"CLI_TEST_FOREIGN_CONFIG_ENV" opaque:""`

	// Settings is looked into as usual, since its opaque tag is turned off.
	Settings CompatibleSettings `opaque:"false"`
}

func (cmd CommandWithOpaqueFields) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithInvalidOpaqueTagValue struct {
	Callback[CommandWithInvalidOpaqueTagValue]

	Settings CompatibleSettings `opaque:"maybe"`
}

func (cmd CommandWithInvalidOpaqueTagValue) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

// RetryPolicy has no cli tags of its own, so it can only be configured as a whole, with a JSON value.
type RetryPolicy struct {
	Max     int
	Backoff time.Duration
}

// DBConfig follows the cli conventions, and it can be configured as a whole with a JSON value too.
type DBConfig struct {
	Host string `flag:"db-host" env:"CLI_TEST_DB_HOST_ENV" default:"localhost"`
	Port int    `flag:"db-port" env:"CLI_TEST_DB_PORT_ENV" default:"5432"`
}

// AuthConfig has a required inner field, which can be provided either by its own input or as part of a JSON value.
type AuthConfig struct {
	Token string `flag:"auth-token" env:"CLI_TEST_AUTH_TOKEN_ENV" required:"true"`
}

type CommandWithStructInputs struct {
	Callback[CommandWithStructInputs]

	Retry RetryPolicy `flag:"retry" env:"CLI_TEST_RETRY_ENV"`
	DB    DBConfig    `flag:"db" env:"CLI_TEST_DB_ENV"`
	Auth  AuthConfig  `flag:"auth" env:"CLI_TEST_AUTH_ENV"`
}

func (cmd CommandWithStructInputs) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

// ReplicaConfig has inner defaults and required inner fields.
type ReplicaConfig struct {
	Host  string `flag:"replica-host" env:"CLI_TEST_REPLICA_HOST_ENV" default:"localhost"`
	Port  int    `flag:"replica-port" env:"CLI_TEST_REPLICA_PORT_ENV" default:"5432"`
	User  string `flag:"replica-user" env:"CLI_TEST_REPLICA_USER_ENV" required:"true"`
	Token string `flag:"replica-token" env:"CLI_TEST_REPLICA_TOKEN_ENV" required:"true"`
}

// CommandWithStructInputDefault puts a JSON value into a default tag. Horrifying, but possible.
// The default JSON overwrites an inner default (Host), satisfies a required inner field (Token),
// and lacks an inner field with a default (Port) and a required inner field (User).
type CommandWithStructInputDefault struct {
	Callback[CommandWithStructInputDefault]

	Replica ReplicaConfig `flag:"replica" env:"CLI_TEST_REPLICA_ENV" default:"{\"Host\":\"replica.internal\",\"Token\":\"from-default\"}"`
}

func (cmd CommandWithStructInputDefault) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
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

func makePattern(t *testcase.T) string {
	for {
		pattern := t.Random.StringNWithCharset(t.Random.IntBetween(1, 10), random.Charset())
		// a pattern should not start like it is a flag
		// Else the test coverage in the future can be extended
		// if there is strong reason to this
		pattern = strings.TrimLeft(pattern, "-")
		if len(pattern) == 0 {
			continue
		}
		return pattern
	}
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

var _ cli.HelpSummary = (*CommandE2E)(nil)

func (cmd CommandE2E) Summary() string { return "E2E command summary" }

var _ cli.HelpDescription = (*CommandE2E)(nil)

func (cmd CommandE2E) Description() string { return "E2E command description" }

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

type CommandWithSliceFlag struct {
	Callback[CommandWithSliceFlag]
	Flag []string `flag:"v" sep:","`
}

func (cmd CommandWithSliceFlag) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithOptArg[T any] struct {
	Callback[CommandWithOptArg[T]]
	Arg T `arg:"0" opt:"true"`
}

func (cmd CommandWithOptArg[T]) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithOptionalEnvVariable struct {
	Callback[CommandWithOptionalEnvVariable]
	V string `env:"V" opt:"true"`
}

func (cmd CommandWithOptionalEnvVariable) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
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

func TestServeCLI_handlerWithOptionalEnvVariable(t *testing.T) {
	t.Run("with env var set", func(tt *testing.T) {
		t := testcase.NewT(tt)
		testcase.SetEnv(t, "V", "test-value")

		var ran bool
		cmd := CommandWithOptionalEnvVariable{Callback: func(v CommandWithOptionalEnvVariable, w cli.ResponseWriter, r *cli.Request) {
			ran = true
			w.ExitCode(42)
			w.Write([]byte("bar"))
		}}

		rr := &cli.ResponseRecorder{}
		req := cli.Request{Body: &bytes.Buffer{}}
		cli.ServeCLI(&cmd, rr, &req)
		assert.Equal(t, rr.Code, 42)
		assert.Equal(t, rr.Out.String(), "bar")
		assert.True(t, ran)
	})

	t.Run("with env var unset", func(t *testing.T) {
		testcase.UnsetEnv(t, "V")

		var ran bool

		cmd := CommandWithOptionalEnvVariable{Callback: func(v CommandWithOptionalEnvVariable, w cli.ResponseWriter, r *cli.Request) {
			ran = true
			w.ExitCode(42)
			w.Write([]byte("foo"))
		}}

		rr := &cli.ResponseRecorder{}
		req := cli.Request{Body: &bytes.Buffer{}}
		cli.ServeCLI(&cmd, rr, &req)
		assert.Equal(t, rr.Code, 42)
		assert.Equal(t, rr.Out.String(), "foo")
		assert.True(t, ran)
	})

	t.Run("with opt env var provided as empty string, cli should considered it as unset", func(t *testing.T) {
		testcase.SetEnv(t, "V", "")

		var ran bool
		cmd := CommandWithOptionalEnvVariable{Callback: func(v CommandWithOptionalEnvVariable, w cli.ResponseWriter, r *cli.Request) {
			ran = true
			w.ExitCode(42)
			w.Write([]byte("foo"))
		}}

		rr := &cli.ResponseRecorder{}
		req := cli.Request{Body: &bytes.Buffer{}}
		cli.ServeCLI(&cmd, rr, &req)

		assert.Equal(t, rr.Code, 42)
		assert.Equal(t, rr.Out.String(), "foo")
		assert.True(t, ran)
	})

	t.Run("handler which embeds a struct that has optional env variables", func(t *testing.T) {
		t.Run("and it is populated", func(t *testing.T) {
			value := "foobarbaz"
			testcase.SetEnv(t, "X", value)

			var ran bool

			cmd := CommandWithEmbeddedOptionalEnvVariable{Callback: func(v CommandWithEmbeddedOptionalEnvVariable, w cli.ResponseWriter, r *cli.Request) {
				ran = true
				w.ExitCode(42)
				assert.Should(t).Equal(value, v.StructWithOptionalEnvVariable.X)
				w.Write([]byte("foo"))
			}}

			rr := &cli.ResponseRecorder{}
			req := cli.Request{Body: &bytes.Buffer{}}
			cli.ServeCLI(&cmd, rr, &req)
			assert.Equal(t, rr.Code, 42)
			assert.Equal(t, rr.Out.String(), "foo")
			assert.True(t, ran)
		})
		t.Run("and it is unset", func(t *testing.T) {
			testcase.UnsetEnv(t, "X")

			var ran bool

			cmd := CommandWithEmbeddedOptionalEnvVariable{Callback: func(v CommandWithEmbeddedOptionalEnvVariable, w cli.ResponseWriter, r *cli.Request) {
				ran = true
				w.ExitCode(42)
				w.Write([]byte("foo"))
			}}

			rr := &cli.ResponseRecorder{}
			req := cli.Request{Body: &bytes.Buffer{}}
			cli.ServeCLI(&cmd, rr, &req)
			assert.Equal(t, rr.Code, 42)
			assert.Equal(t, rr.Out.String(), "foo")
			assert.True(t, ran)
		})
	})
}

type CommandWithEmbeddedOptionalEnvVariable struct {
	Callback[CommandWithEmbeddedOptionalEnvVariable]
	StructWithOptionalEnvVariable
}

type StructWithOptionalEnvVariable struct {
	X string `env:"X" opt:"true"`
}

func (cmd CommandWithEmbeddedOptionalEnvVariable) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
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
	_ = act

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
		// o := act(t)
		// assert.True(t, o.Goexit)
		// assert.NotNil(t, code.Get(t))
		// assert.Equal(t, *code.Get(t), handlerExitCode.Get(t))
	})

	s.Then("handler response body is propagated towards STDOUT", func(t *testcase.T) {

	})

}

func Test_help_path(t *testing.T) {
	t.Run("top routes", func(t *testing.T) {
		var mux cli.ServeMux
		mux.Handle("cmd", SubCommand{})

		t.Run("mux help output displays path", func(t *testing.T) {
			rr := &cli.ResponseRecorder{}
			req := &cli.Request{
				Args: []string{"--help"},
				Body: &bytes.Buffer{},
			}

			testcase.OnFail(t, func() {
				t.Logf("\noutput:\n\n%s\n", rr.Out.String())
			})

			cli.ServeCLI(&mux, rr, req)
			assert.MatchRegexp(t, rr.Out.String(), `Usage: [^\s]+\n`)
			assert.Contains(t, rr.Out.String(), "Commands:\n - cmd\n")
		})

		t.Run("sub command help output displays path", func(t *testing.T) {
			rr := &cli.ResponseRecorder{}
			req := &cli.Request{
				Args: []string{"cmd", "--help"},
				Body: &bytes.Buffer{},
			}

			testcase.OnFail(t, func() {
				t.Logf("\noutput:\n\n%s\n", rr.Out.String())
			})

			cli.ServeCLI(&mux, rr, req)
			assert.MatchRegexp(t, rr.Out.String(), `Usage: [^\s]+ cmd`)
			assert.NotContains(t, rr.Out.String(), "Commands:")
		})
	})
	t.Run("sub routes", func(t *testing.T) {
		var mux, foo, bar, baz cli.ServeMux
		mux.Handle("foo", &foo)
		foo.Handle("bar", &bar)
		bar.Handle("baz", &baz)
		baz.Handle("cmd", SubCommand{})

		t.Run("sub mux help output displays path", func(t *testing.T) {
			rr := &cli.ResponseRecorder{}
			req := &cli.Request{
				Args: []string{"foo", "bar", "baz", "--help"},
				Body: &bytes.Buffer{},
			}

			testcase.OnFail(t, func() {
				t.Logf("\noutput:\n\n%s\n", rr.Out.String())
			})

			cli.ServeCLI(&mux, rr, req)
			assert.MatchRegexp(t, rr.Out.String(), `Usage: [^\s]+ foo bar baz`)
			assert.Contains(t, rr.Out.String(), "Commands:\n - cmd\n")
		})

		t.Run("sub command help output displays path", func(t *testing.T) {
			rr := &cli.ResponseRecorder{}
			req := &cli.Request{
				Args: []string{"foo", "bar", "baz", "cmd", "--help"},
				Body: &bytes.Buffer{},
			}

			testcase.OnFail(t, func() {
				t.Logf("\noutput:\n\n%s\n", rr.Out.String())
			})

			cli.ServeCLI(&mux, rr, req)
			assert.MatchRegexp(t, rr.Out.String(), `Usage: [^\s]+ foo bar baz cmd`)
			assert.NotContains(t, rr.Out.String(), "Commands:")
		})
	})
}

type SliceFlagWithSeperatorOption struct {
	Callback[SliceFlagWithSeperatorOption]
	VS []string `flag:"vs" separator:"/"`
}

func (cmd SliceFlagWithSeperatorOption) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

func Test_sliceFlagType_supportSeperatorOption(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		var mux cli.ServeMux
		mux.Handle("cmd", SliceFlagWithSeperatorOption{})

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: []string{"cmd", "--help"},
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Contains(t, rr.Out.String(), `(seperator: /)`)
	})

	t.Run("value provided with correct separator", func(t *testing.T) {
		var mux cli.ServeMux
		var out SliceFlagWithSeperatorOption
		mux.Handle("cmd", SliceFlagWithSeperatorOption{
			Callback: func(v SliceFlagWithSeperatorOption, w cli.ResponseWriter, r *cli.Request) {
				out = v
			},
		})

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: []string{"cmd", "-vs", "foo/bar/baz"},
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, out.VS, []string{"foo", "bar", "baz"})
	})

	t.Run("value provided with incorrect separator", func(t *testing.T) {
		var mux cli.ServeMux
		var out SliceFlagWithSeperatorOption
		mux.Handle("cmd", SliceFlagWithSeperatorOption{
			Callback: func(v SliceFlagWithSeperatorOption, w cli.ResponseWriter, r *cli.Request) {
				out = v
			},
		})

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: []string{"cmd", "-vs", "foo,bar,baz"},
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, out.VS, []string{"foo,bar,baz"})
	})
}

func Test_sliceFlagType_supportInputFlagRepetition(t *testing.T) {
	t.Run("mixed use", func(t *testing.T) {
		var mux cli.ServeMux
		var out CommandWithSliceFlag
		mux.Handle("cmd", CommandWithSliceFlag{Callback: func(v CommandWithSliceFlag, w cli.ResponseWriter, r *cli.Request) {
			out = v
		}})

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: []string{"cmd", "-v", "foo,bar", "-v", "baz"},
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, out.Flag, []string{"foo", "bar", "baz"})
	})
}

type CommandWithVariadicArg1 struct {
	Callback[CommandWithVariadicArg1]

	ArgFirst string   `arg:"0"`
	ArgsRest []string `arg:"1:"`
}

func (cmd CommandWithVariadicArg1) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithVariadicArg2 struct {
	Callback[CommandWithVariadicArg2]

	Args []string `arg:":"`
}

func (cmd CommandWithVariadicArg2) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithVariadicArg3 struct {
	Callback[CommandWithVariadicArg3]

	Args []string `arg:":" len:"2<"`
}

func (cmd CommandWithVariadicArg3) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithVariadicArgRange struct {
	Callback[CommandWithVariadicArgRange]

	First     string   `arg:"0"`
	Middle    []string `arg:"1:3"`
	Remaining []string `arg:"3:" opt:"T"`
}

func (cmd CommandWithVariadicArgRange) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithVariadicArgRange02 struct {
	Callback[CommandWithVariadicArgRange02]

	First     string   `arg:"0"`
	Middle    []string `arg:"1:2"`
	Remaining []string `arg:"2:" opt:"T"`
}

func (cmd CommandWithVariadicArgRange02) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithVariadicArgRange03 struct {
	Callback[CommandWithVariadicArgRange03]

	VS []string `arg:"0:5"`
}

func (cmd CommandWithVariadicArgRange03) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

type CommandWithVariadicArgRange04 struct {
	Callback[CommandWithVariadicArgRange04]

	VS []string `arg:"0:5" opt:"T" required:"false"`
}

func (cmd CommandWithVariadicArgRange04) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Callback.Call(cmd, w, r)
}

func Test_variadicArgInput(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test(`args with slice expression to capture variadic arguments after n th index -- arg:"n:" -- for example arg:"1:`, func(t *testcase.T) {
		var mux cli.ServeMux
		var out CommandWithVariadicArg1
		var lastRequest *cli.Request
		mux.Handle("cmd", CommandWithVariadicArg1{Callback: func(v CommandWithVariadicArg1, w cli.ResponseWriter, r *cli.Request) {
			out = v
			lastRequest = r
		}})

		var first = t.Random.String()
		var rest = random.Slice(t.Random.IntBetween(3, 7), t.Random.String)

		var args []string = []string{"cmd", "--"}
		args = append(args, first)
		args = append(args, rest...)

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: args,
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, out.ArgFirst, first)
		assert.Equal(t, out.ArgsRest, rest)
		assert.NotNil(t, lastRequest)
		assert.Empty(t, lastRequest.Args)
	})

	s.Test(`args with slice expression to capture all variadic arguments (basically passthrough) -- arg:":"`, func(t *testcase.T) {
		var mux cli.ServeMux
		var out CommandWithVariadicArg2
		var lastRequest *cli.Request
		mux.Handle("cmd", CommandWithVariadicArg2{Callback: func(v CommandWithVariadicArg2, w cli.ResponseWriter, r *cli.Request) {
			out = v
			lastRequest = r
		}})

		var cmdArgs = random.Slice(t.Random.IntBetween(3, 7), t.Random.String)

		var args []string = []string{"cmd", "--"}
		args = append(args, cmdArgs...)

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: args,
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, out.Args, cmdArgs)
		assert.NotNil(t, lastRequest)
		assert.Empty(t, lastRequest.Args)
	})

	s.Context(`arg slice expression combined with validation should make a good synenergy, for example argument length enforcement`, func(s *testcase.Spec) {
		s.Test("happy", func(t *testcase.T) {
			var mux cli.ServeMux
			var out CommandWithVariadicArg3
			var lastRequest *cli.Request
			mux.Handle("cmd", CommandWithVariadicArg3{Callback: func(v CommandWithVariadicArg3, w cli.ResponseWriter, r *cli.Request) {
				out = v
				lastRequest = r
			}})

			var cmdArgs = random.Slice(t.Random.IntBetween(3, 7), t.Random.String)

			var args []string = []string{"cmd", "--"}
			args = append(args, cmdArgs...)

			rr := &cli.ResponseRecorder{}
			req := &cli.Request{
				Args: args,
				Body: &bytes.Buffer{},
			}

			debugRecorderResult(t, rr)

			cli.ServeCLI(&mux, rr, req)
			assert.Equal(t, out.Args, cmdArgs)
			assert.NotNil(t, lastRequest)
			assert.Empty(t, lastRequest.Args)
		})

		s.Test("rainy", func(t *testcase.T) {
			var mux cli.ServeMux
			var ran bool
			mux.Handle("cmd", CommandWithVariadicArg3{Callback: func(v CommandWithVariadicArg3, w cli.ResponseWriter, r *cli.Request) {
				ran = true
			}})

			var cmdArgs = random.Slice(t.Random.IntBetween(0, 2), t.Random.String)

			var args []string = []string{"cmd", "--"}
			args = append(args, cmdArgs...)

			rr := &cli.ResponseRecorder{}
			req := &cli.Request{
				Args: args,
				Body: &bytes.Buffer{},
			}

			debugRecorderResult(t, rr)

			cli.ServeCLI(&mux, rr, req)
			assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
			assert.False(t, ran, "expected that the Command was not executed due to validation issue")
		})
	})

	s.Test(`args with range expression to capture arguments between two indices -- arg:"1:3"`, func(t *testcase.T) {
		var mux cli.ServeMux
		var out CommandWithVariadicArgRange
		var lastRequest *cli.Request
		mux.Handle("cmd", CommandWithVariadicArgRange{Callback: func(v CommandWithVariadicArgRange, w cli.ResponseWriter, r *cli.Request) {
			out = v
			lastRequest = r
		}})

		// args[0] -> First
		// args[1:3] -> Middle (indices 1 and 2)
		// args[3:] -> Remaining
		var first = t.Random.String()
		var middleFirst = t.Random.String()
		var middleSecond = t.Random.String()
		var remaining = random.Slice(t.Random.IntBetween(0, 4), t.Random.String)

		args := []string{"cmd", "--", first, middleFirst, middleSecond}
		args = append(args, remaining...)

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: args,
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, out.First, first)
		assert.Equal(t, out.Middle, []string{middleFirst, middleSecond})
		assert.ContainsExactly(t, out.Remaining, remaining)
		assert.NotNil(t, lastRequest)
		assert.Empty(t, lastRequest.Args)
	})

	s.Test(`args with range expression capturing single middle element -- arg:"1:2"`, func(t *testcase.T) {
		var mux cli.ServeMux
		var out CommandWithVariadicArgRange02
		var lastRequest *cli.Request
		mux.Handle("cmd", CommandWithVariadicArgRange02{Callback: func(v CommandWithVariadicArgRange02, w cli.ResponseWriter, r *cli.Request) {
			out = v
			lastRequest = r
		}})

		var first = t.Random.String()
		var second = t.Random.String()
		var remaining = random.Slice(t.Random.IntBetween(1, 4), t.Random.String)

		args := []string{"cmd", "--", first, second}
		args = append(args, remaining...)

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: args,
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, out.First, first)
		assert.Equal(t, out.Middle, []string{second})
		assert.Equal(t, out.Remaining, remaining)
		assert.NotNil(t, lastRequest)
		assert.Empty(t, lastRequest.Args)
	})

	s.Test("when the expected argument count is less than the variadic argument slicing range then it should report missing arguments", func(t *testcase.T) {
		var mux cli.ServeMux
		mux.Handle("cmd", CommandWithVariadicArgRange03{Callback: func(v CommandWithVariadicArgRange03, w cli.ResponseWriter, r *cli.Request) {}})

		// 3 element is less than the expected args[0:5]
		args := []string{"cmd", "--", t.Random.String(), t.Random.String(), t.Random.String()}

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: args,
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
		assert.MatchRegexp(t, rr.Err.String(), `Usage: [^\s]+ VS1 VS2 VS3 VS4 VS5`,
			"expected that the usage showcase how many values expected for the VS input arg field")
		assert.Contains(t, rr.Err.String(), "too few arguments for VS",
			"expected that the error output points out that too few argument is provided for VS")
	})

	s.Test("when optional variadic argument is simply not provided at all, then it should be accepted without an issue", func(t *testcase.T) {
		var mux cli.ServeMux

		var ran bool
		mux.Handle("cmd", CommandWithVariadicArgRange04{Callback: func(v CommandWithVariadicArgRange04, w cli.ResponseWriter, r *cli.Request) {
			ran = true
		}})

		// 3 element is less than the expected args[0:5]
		args := []string{"cmd"}

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: args,
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, rr.Code, cli.ExitCodeOK)
		assert.True(t, ran)
	})

	s.Test("when optional variadic argument is partially provided, then it is considered as bad request", func(t *testcase.T) {
		var mux cli.ServeMux

		var ran bool
		mux.Handle("cmd", CommandWithVariadicArgRange04{Callback: func(v CommandWithVariadicArgRange04, w cli.ResponseWriter, r *cli.Request) {
			ran = true
		}})

		// 3 element is less than the expected args[0:5]
		args := []string{"cmd", "--", t.Random.String(), t.Random.String(), t.Random.String()}

		rr := &cli.ResponseRecorder{}
		req := &cli.Request{
			Args: args,
			Body: &bytes.Buffer{},
		}

		debugRecorderResult(t, rr)

		cli.ServeCLI(&mux, rr, req)
		assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
		assert.False(t, ran, "expected that the command's ServeCLI was not executed due to bad input")
		assert.MatchRegexp(t, rr.Err.String(), `Usage: [^\s]+ VS1 VS2 VS3 VS4 VS5`,
			"expected that the usage showcase how many values expected for the VS input arg field")
		assert.Contains(t, rr.Err.String(), "too few arguments for VS",
			"expected that the error output points out that too few argument is provided for VS")

	})
}

type NonSliceTypeWithVariadicArgTag struct {
	V string `arg:":"`
}

func (NonSliceTypeWithVariadicArgTag) ServeCLI(w cli.ResponseWriter, r *cli.Request) {}

func debugRecorderResult(tb testing.TB, rr *cli.ResponseRecorder) {
	testcase.OnFail(tb, func() {
		tb.Logf("\noutput:\n\n%s\n", rr.Out.String())
		tb.Logf("\nerror:\n\n%s\n", rr.Err.String())
	})
}

func TestResponseRecorder(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := testcase.Let(s, func(t *testcase.T) *cli.ResponseRecorder {
		return &cli.ResponseRecorder{}
	})

	s.Describe("#CombinedOutput", func(s *testcase.Spec) {
		act := func(t *testcase.T) []byte {
			return subject.Get(t).CombinedOutput()
		}

		s.When("both stdout and stderr are empty", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.Empty(t, subject.Get(t).Out.Bytes())
				assert.Empty(t, subject.Get(t).Err.Bytes())
			})

			s.Then("returns an empty byte slice", func(t *testcase.T) {
				result := act(t)
				assert.Equal(t, len(result), 0)
				assert.True(t, len(result) == 0 || result == nil)
			})
		})

		s.When("stdout has content", func(s *testcase.Spec) {
			expected := let.String(s)

			s.Before(func(t *testcase.T) {
				subject.Get(t).Out.WriteString(expected.Get(t))
			})

			s.Then("returns only stdout content", func(t *testcase.T) {
				got := act(t)

				assert.Equal(t, string(got), expected.Get(t))
			})
		})

		s.When("stderr has content", func(s *testcase.Spec) {
			expected := let.String(s)

			s.Before(func(t *testcase.T) {
				subject.Get(t).Out.WriteString(expected.Get(t))
			})

			s.Then("returns only stderr content", func(t *testcase.T) {
				got := act(t)

				assert.Equal(t, string(got), expected.Get(t))
			})
		})

		s.When("both stdout and stderr have content", func(s *testcase.Spec) {
			var (
				expectedStdout = let.UUID(s)
				expectedStderr = let.UUID(s)
			)

			s.Before(func(t *testcase.T) {
				subject.Get(t).Out.WriteString(expectedStdout.Get(t))
				subject.Get(t).Err.WriteString(expectedStderr.Get(t))
			})

			s.Then("returns stdout followed by stderr", func(t *testcase.T) {
				result := act(t)

				expected := expectedStdout.Get(t) + expectedStderr.Get(t)
				assert.Equal(t, string(result), expected)

				stdoutIdx := bytes.Index(result, []byte(expectedStdout.Get(t)))
				stderrIdx := bytes.Index(result, []byte(expectedStderr.Get(t)))
				assert.True(t, stdoutIdx >= 0 && stderrIdx >= 0)
				assert.True(t, stdoutIdx < stderrIdx)
			})
		})
	})

	s.Describe("#Stdout", func(s *testcase.Spec) {
		act := func(t *testcase.T) io.Writer {
			return subject.Get(t).Stdout()
		}

		s.Then("it returns the writer that records into ResponseRecorder.Out", func(t *testcase.T) {
			exp := t.Random.String()

			_, err := io.WriteString(act(t), exp)
			assert.NoError(t, err)

			assert.Equal(t, subject.Get(t).Out.String(), exp)
			assert.Empty(t, subject.Get(t).Err.String())
		})
	})
}

func TestStdResponse(t *testing.T) {
	s := testcase.NewSpec(t)

	subject := let.Var(s, func(t *testcase.T) *cli.StdResponse {
		return &cli.StdResponse{}
	})

	s.Describe("#Stdout", func(s *testcase.Spec) {
		act := func(t *testcase.T) io.Writer {
			return subject.Get(t).Stdout()
		}

		s.Then("it returns the standard output of the process", func(t *testcase.T) {
			assert.True(t, act(t) == io.Writer(os.Stdout))
		})
	})
}

func TestError(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		rr = let.Var(s, func(t *testcase.T) *cli.ResponseRecorder {
			return &cli.ResponseRecorder{}
		})
		w = let.Var(s, func(t *testcase.T) cli.ResponseWriter {
			return rr.Get(t)
		})
		message = let.String(s)
		code    = let.Int(s)
	)
	act := let.Act0(func(t *testcase.T) {
		cli.Error(w.Get(t), message.Get(t), code.Get(t))
	})

	s.Then("it won't stop the execution of the goroutine", func(t *testcase.T) {
		o := sandbox.Run(func() { act(t) })

		assert.True(t, o.OK)
		assert.False(t, o.Goexit)
		assert.False(t, o.Panic)
	})

	s.Then("error will writes error message into response writer", func(t *testcase.T) {
		act(t)

		output := string(rr.Get(t).CombinedOutput())
		assert.Contains(t, output, message.Get(t))
	})

	s.Then("it will set exit code in the response writer", func(t *testcase.T) {
		act(t)

		assert.Equal(t, code.Get(t), rr.Get(t).Code)
	})

	s.When("response writer supports cli.ErrorWriter", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			_, ok := w.Get(t).(cli.ErrorWriter)
			assert.True(t, ok)
		})

		s.Then("ErrorWriter will be used to write response", func(t *testcase.T) {
			act(t)

			assert.Contains(t, rr.Get(t).Err.String(), message.Get(t))
			assert.Empty(t, rr.Get(t).Out.String())
		})
	})

	s.When("response writer lack support for cli.ErrorWriter", func(s *testcase.Spec) {
		w.Let(s, func(t *testcase.T) cli.ResponseWriter {
			return WithoutErrorWriter{W: rr.Get(t)}
		})
		s.Before(func(t *testcase.T) {
			_, ok := w.Get(t).(cli.ErrorWriter)
			assert.False(t, ok)
		})

		s.Then("Write will be used to write response", func(t *testcase.T) {
			act(t)

			assert.Contains(t, rr.Get(t).Out.String(), message.Get(t))
			assert.Empty(t, rr.Get(t).Err.String())
		})
	})
}

type WithoutErrorWriter struct {
	W cli.ResponseWriter
}

func (woEW WithoutErrorWriter) ExitCode(n int) {
	woEW.W.ExitCode(n)
}

func (woEW WithoutErrorWriter) Write(p []byte) (n int, err error) {
	return woEW.W.Write(p)
}

func TestMain_signalInterrupt(tt *testing.T) {
	t := testcase.NewT(tt)

	var handlerDone int32
	h := cli.HandlerFunc(func(w cli.ResponseWriter, r *cli.Request) {
		<-r.Context().Done()
		atomic.StoreInt32(&handlerDone, 1)
	})

	logger.Stub(t)
	osint.StubExit(t, func(code int) {})
	osint.StubArgs(t, []string{"cmd"})

	var (
		notifyMutex sync.Mutex
		sigch       chan<- os.Signal
		sigs        []os.Signal
	)
	defer signalint.StubNotify(func(c chan<- os.Signal, sig ...os.Signal) {
		notifyMutex.Lock()
		defer notifyMutex.Unlock()
		sigch = c
		sigs = sig
	})()
	notified := func() (chan<- os.Signal, []os.Signal) {
		notifyMutex.Lock()
		defer notifyMutex.Unlock()
		return sigch, sigs
	}

	var mainDone int32
	go func() {
		sandbox.Run(func() {
			cli.Main(context.Background(), h)
		})
		atomic.StoreInt32(&mainDone, 1)
	}()

	t.Eventually(func(t *testcase.T) {
		c, sigs := notified()
		assert.NotEmpty(t, sigs)
		assert.NotNil(t, c)
	})
	notifyCh, notifySigs := notified()

	w := assert.NotWithin(t, time.Millisecond, func(ctx context.Context) {
		for atomic.LoadInt32(&handlerDone) == 0 {
		}
	})

	select {
	case notifyCh <- random.Pick(t.Random, notifySigs...):
	case <-t.Context().Done():
	case <-time.After(time.Second):
		t.FailNow()
	}

	assert.Within(t, 100*time.Millisecond, func(ctx context.Context) {
		w.Wait()
	})

	t.Eventually(func(t *testcase.T) {
		assert.Equal(t, atomic.LoadInt32(&handlerDone), 1)
		assert.Equal(t, atomic.LoadInt32(&mainDone), 1)
	})
}
