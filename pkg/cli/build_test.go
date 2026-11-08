package cli_test

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestBuild(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		args = let.Var(s, func(t *testcase.T) []string {
			return nil
		})
		request = let.Var(s, func(t *testcase.T) *cli.Request {
			return &cli.Request{Args: args.Get(t)}
		})
	)

	s.Before(func(t *testcase.T) {
		t.OnFail(func() { t.Log("args:", args.Get(t)) })
	})

	s.Describe("flags and args", func(s *testcase.Spec) {
		var (
			ptr = let.Var(s, func(t *testcase.T) *CommandE2E {
				return &CommandE2E{}
			})
			strFlag = let.StringNC(s, 8, random.CharsetAlpha())
			intFlag = let.IntB(s, 1, 1000)
			strArg  = let.StringNC(s, 8, random.CharsetAlpha())
			intArg  = let.IntB(s, 1, 1000)
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return []string{
				/* flags */ "-str", strFlag.Get(t), "-int", strconv.Itoa(intFlag.Get(t)), "-bool=true", "-sbool", "-fbool=0",
				/* args */ strArg.Get(t), strconv.Itoa(intArg.Get(t)), "True",
			}
		})

		s.Then("the given flags are set on the handler behind the pointer", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).Flag1, strFlag.Get(t))
			assert.Equal(t, ptr.Get(t).Flag3, intFlag.Get(t))
			assert.Equal(t, ptr.Get(t).Flag4, true)
			assert.Equal(t, ptr.Get(t).Flag5, true)
			assert.Equal(t, ptr.Get(t).Flag6, false)
		})

		s.Then("the flags which are not given get their default tag value", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).Flag2, "defval")
		})

		s.Then("the given args are set on the handler behind the pointer, by their index", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).Arg1, strArg.Get(t))
			assert.Equal(t, ptr.Get(t).Arg2, intArg.Get(t))
			assert.Equal(t, ptr.Get(t).Arg3, true)
		})
	})

	s.Describe("required flag", func(s *testcase.Spec) {
		var (
			ptr = let.Var(s, func(t *testcase.T) *CommandWithFlagWithRequired[string] {
				return &CommandWithFlagWithRequired[string]{}
			})
			value = let.StringNC(s, 8, random.CharsetAlpha())
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return []string{"-flag", value.Get(t)}
		})

		s.Then("the given flag value is set on the handler behind the pointer", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).Flag, value.Get(t))
		})

		s.When("the flag is not given", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return nil
			})

			s.Then("it returns an invalid input error about the missing flag", func(t *testcase.T) {
				_, err := act(t)

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				assert.Error(t, eii.Err)
				assert.Contains(t, eii.Err.Error(), "missing input")
			})

			s.Then("the error message names the flag, and tells the cause", func(t *testcase.T) {
				_, err := act(t)

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				assert.Equal(t, err.Error(), "[ErrInvalidInput] -flag: "+eii.Err.Error())
			})

			s.And("the handler already has a value for it, like a dependency injected default", func(s *testcase.Spec) {
				injected := let.StringNC(s, 8, random.CharsetAlpha())
				ptr.Let(s, func(t *testcase.T) *CommandWithFlagWithRequired[string] {
					return &CommandWithFlagWithRequired[string]{Flag: injected.Get(t)}
				})

				s.Then("the required flag is satisfied, and the value is kept", func(t *testcase.T) {
					_, err := act(t)
					assert.NoError(t, err)

					assert.Equal(t, ptr.Get(t).Flag, injected.Get(t))
				})
			})
		})
	})

	s.Describe("required arg", func(s *testcase.Spec) {
		var (
			ptr = let.Var(s, func(t *testcase.T) *CommandWithArgWithRequired[string] {
				return &CommandWithArgWithRequired[string]{}
			})
			value = let.StringNC(s, 8, random.CharsetAlpha())
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return []string{value.Get(t)}
		})

		s.Then("the given arg value is set on the handler behind the pointer", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).Arg, value.Get(t))
		})

		s.When("the arg is not given", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return nil
			})

			s.Then("it returns an invalid input error about the missing arg", func(t *testcase.T) {
				_, err := act(t)

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				assert.Error(t, eii.Err)
				assert.Contains(t, eii.Err.Error(), "missing input")
			})

			s.Then("the error message names the arg position, and tells the cause", func(t *testcase.T) {
				_, err := act(t)

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				assert.Equal(t, err.Error(), "[ErrInvalidInput] arg[0]: "+eii.Err.Error())
			})

			s.And("the handler already has a value for it, like a dependency injected default", func(s *testcase.Spec) {
				injected := let.StringNC(s, 8, random.CharsetAlpha())
				ptr.Let(s, func(t *testcase.T) *CommandWithArgWithRequired[string] {
					return &CommandWithArgWithRequired[string]{Arg: injected.Get(t)}
				})

				s.Then("the required arg is satisfied, and the value is kept", func(t *testcase.T) {
					_, err := act(t)
					assert.NoError(t, err)

					assert.Equal(t, ptr.Get(t).Arg, injected.Get(t))
				})
			})
		})
	})

	s.Describe("env", func(s *testcase.Spec) {
		const envKey = "FLAGNAME" // the env tag of CommandWithFlagWithEnv.Flag

		var (
			ptr = let.Var(s, func(t *testcase.T) *CommandWithFlagWithEnv[string] {
				return &CommandWithFlagWithEnv[string]{}
			})
			envValue = let.StringNC(s, 8, random.CharsetAlpha())
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		s.Before(func(t *testcase.T) {
			testcase.SetEnv(t, envKey, envValue.Get(t))
		})

		s.Then("the env var value is set on the handler behind the pointer", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).Flag, envValue.Get(t))
		})

		s.When("the env var is not set", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.UnsetEnv(t, envKey)
			})

			s.Then("the field is left empty, without an error", func(t *testcase.T) {
				_, err := act(t)
				assert.NoError(t, err)

				assert.Empty(t, ptr.Get(t).Flag)
			})
		})
	})

	s.Describe("env only input", func(s *testcase.Spec) {
		const envKey = "CLI_TEST_TRUE_DEFAULT_BOOL_ENV" // the env tag of CommandWithTrueDefaultBools.Env

		var (
			ptr = let.Var(s, func(t *testcase.T) *CommandWithTrueDefaultBools {
				return &CommandWithTrueDefaultBools{}
			})
			envValue = let.VarOf(s, "false")
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		s.Before(func(t *testcase.T) {
			testcase.SetEnv(t, envKey, envValue.Get(t))
		})

		s.Then("the env var value is set on the handler behind the pointer", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).Env, false)
		})

		s.When("the env var value can't be parsed", func(s *testcase.Spec) {
			envValue.Let(s, func(t *testcase.T) string {
				return t.Random.StringNC(8, random.CharsetAlpha())
			})

			s.Then("it returns an invalid input error, whose message names the env var, and tells the cause", func(t *testcase.T) {
				_, err := act(t)

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				assert.Error(t, eii.Err)
				assert.Equal(t, err.Error(), "[ErrInvalidInput] $"+envKey+": "+eii.Err.Error())
			})
		})
	})

	s.Describe("enum", func(s *testcase.Spec) {
		var (
			ptr = let.Var(s, func(t *testcase.T) *CommandWithFlagWithEnum {
				return &CommandWithFlagWithEnum{}
			})
			value = let.OneOf(s, "foo", "bar", "baz") // the enum tag values of CommandWithFlagWithEnum.Flag
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return []string{"-flag", value.Get(t)}
		})

		s.Then("the given enum value is set on the handler behind the pointer", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).Flag, value.Get(t))
		})

		s.When("the flag is not given", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return nil
			})

			s.Then("the field is left empty, without an error", func(t *testcase.T) {
				_, err := act(t)
				assert.NoError(t, err)

				assert.Empty(t, ptr.Get(t).Flag)
			})
		})

		s.When("the given value is not one of the enum values", func(s *testcase.Spec) {
			value.Let(s, func(t *testcase.T) string {
				return t.Random.StringNC(8, random.CharsetAlpha())
			})

			s.Then("it returns an enum user error", func(t *testcase.T) {
				_, err := act(t)

				var got errorkit.UserError
				assert.True(t, errors.As(err, &got))
				assert.Equal(t, got.Code, "enum-error")
			})

			s.Then("the error message names the flag and the invalid value, without a stack trace", func(t *testcase.T) {
				_, err := act(t)

				assert.True(t, strings.HasPrefix(err.Error(), "[ErrInvalidInput] -flag: "))
				assert.Contains(t, err.Error(), value.Get(t))
				assert.NotContains(t, err.Error(), "build_test.go")
			})
		})
	})

	s.Describe("unexported fields", func(s *testcase.Spec) {
		var (
			ptr = let.Var(s, func(t *testcase.T) *CommandWithUnexportedField {
				return &CommandWithUnexportedField{}
			})
			value = let.StringNC(s, 8, random.CharsetAlpha())
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return []string{"-ef", value.Get(t)}
		})

		// What happens to the unexported input fields is deliberately left undefined.
		s.Then("the exported input fields are set, while the unexported ones don't fail the build", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).ExportedFlag, value.Get(t))
		})
	})

	s.Describe("variadic arg tag", func(s *testcase.Spec) {
		ptr := let.Var(s, func(t *testcase.T) *NonSliceTypeWithVariadicArgTag {
			return &NonSliceTypeWithVariadicArgTag{}
		})
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return []string{"foo", "bar", "baz"}
		})

		s.Then("it panics on a non-slice field, as it is a programming error, not an invalid input", func(t *testcase.T) {
			out := assert.Panic(t, func() { _, _ = act(t) })

			assert.Contains(t, fmt.Sprintf("%v", out), `variadic arg tag selector with ":" only supported for []T types`)
		})
	})

	s.Describe("flag collision", func(s *testcase.Spec) {
		ptr := let.Var(s, func(t *testcase.T) *CommandWithFlagCollision {
			return &CommandWithFlagCollision{}
		})
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		s.Then("it panics with the flag and the fields which define it, as it is a programming error", func(t *testcase.T) {
			out := assert.Panic(t, func() { _, _ = act(t) })

			msg := fmt.Sprintf("%v", out)
			assert.Contains(t, msg, "-host")
			assert.Contains(t, msg, "Primary.Host")
			assert.Contains(t, msg, "Replica.Host")
		})
	})

	s.Describe("arg position gap", func(s *testcase.Spec) {
		ptr := let.Var(s, func(t *testcase.T) *CommandWithArgPositionGap {
			return &CommandWithArgPositionGap{}
		})
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		s.Then("it returns an error with the arg field that is out of position, regardless of the inputs", func(t *testcase.T) {
			_, err := act(t)
			assert.Error(t, err)

			assert.Contains(t, err.Error(), "Third")
		})
	})

	s.Describe("range arg", func(s *testcase.Spec) {
		var (
			ptr = let.Var(s, func(t *testcase.T) *CommandWithVariadicArgRange03 {
				return &CommandWithVariadicArgRange03{}
			})
			values = let.Var(s, func(t *testcase.T) []string {
				var vs []string
				for range 5 { // the range of CommandWithVariadicArgRange03.VS is [0:5]
					vs = append(vs, t.Random.StringNC(8, random.CharsetAlpha()))
				}
				return vs
			})
		)
		act := func(t *testcase.T) (cli.Handler, error) {
			return cli.Build(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return values.Get(t)
		})

		s.Then("the given values are set on the handler behind the pointer", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, ptr.Get(t).VS, values.Get(t))
		})

		s.When("the range is given partially", func(s *testcase.Spec) {
			values.Let(s, func(t *testcase.T) []string {
				return values.Super(t)[:t.Random.IntBetween(1, 4)]
			})

			s.Then("it returns an error about the too few arguments", func(t *testcase.T) {
				_, err := act(t)
				assert.Error(t, err)

				assert.Contains(t, err.Error(), "too few arguments for VS")
			})
		})
	})
}
