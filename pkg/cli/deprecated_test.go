package cli_test

import (
	"errors"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func TestConfigureHandler(t *testing.T) {
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
		ptr := let.Var(s, func(t *testcase.T) *CommandE2E {
			return &CommandE2E{}
		})
		act := func(t *testcase.T) error {
			return cli.ConfigureHandler(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return []string{
				/* flags */ "-str", "foo", "-int", "42", "-bool=true", "-sbool", "-fbool=0",
				/* args */ "hello-world", "42", "True",
			}
		})

		s.Then("the handler behind the pointer is configured with the given flags and args, and the default tag values", func(t *testcase.T) {
			assert.NoError(t, act(t))

			cmd := ptr.Get(t)
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
	})

	s.Describe("flag collision", func(s *testcase.Spec) {
		ptr := let.Var(s, func(t *testcase.T) *CommandWithFlagCollision {
			return &CommandWithFlagCollision{}
		})
		act := func(t *testcase.T) error {
			return cli.ConfigureHandler(ptr.Get(t), request.Get(t))
		}

		s.Then("it panics with the flag and the fields which define it, as it is a programming error", func(t *testcase.T) {
			out := assert.Panic(t, func() { _ = act(t) })

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
		act := func(t *testcase.T) error {
			return cli.ConfigureHandler(ptr.Get(t), request.Get(t))
		}

		s.Then("it returns an error with the arg field that is out of position, regardless of the inputs", func(t *testcase.T) {
			err := act(t)
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
		act := func(t *testcase.T) error {
			return cli.ConfigureHandler(ptr.Get(t), request.Get(t))
		}

		args.Let(s, func(t *testcase.T) []string {
			return values.Get(t)
		})

		s.Then("the given values are set on the handler behind the pointer", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, ptr.Get(t).VS, values.Get(t))
		})

		s.When("the range is given partially", func(s *testcase.Spec) {
			values.Let(s, func(t *testcase.T) []string {
				return values.Super(t)[:t.Random.IntBetween(1, 4)]
			})

			s.Then("it returns an error about the too few arguments", func(t *testcase.T) {
				err := act(t)
				assert.Error(t, err)

				assert.Contains(t, err.Error(), "too few arguments for VS")
			})
		})
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
	const envName = "FLAGNAME"
	t.Run("no env var", func(t *testing.T) {
		testcase.UnsetEnv(t, envName)
		cmd := CommandWithFlagWithEnv[string]{}
		err := cli.ConfigureHandler(&cmd, &cli.Request{})
		assert.NoError(t, err)
		assert.Empty(t, cmd.Flag)
	})

	t.Run("with env var", func(t *testing.T) {
		testcase.SetEnv(t, envName, "val")

		cmd := CommandWithFlagWithEnv[string]{}
		err := cli.ConfigureHandler(&cmd, &cli.Request{})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Flag, "val")
	})
}

func TestConfigureHandler_enumIntegration(t *testing.T) {
	t.Run("no enum value", func(t *testing.T) {
		cmd := CommandWithFlagWithEnum{}
		err := cli.ConfigureHandler(&cmd, &cli.Request{Args: []string{}})
		assert.NoError(t, err)
		assert.Empty(t, cmd.Flag)
	})

	t.Run("with valid enum value", func(tt *testing.T) {
		t := testcase.NewT(tt)
		exp := random.Pick(t.Random, "foo", "bar", "baz")

		cmd := CommandWithFlagWithEnum{}
		err := cli.ConfigureHandler(&cmd, &cli.Request{Args: []string{"-flag", exp}})
		assert.NoError(t, err)
		assert.Equal(t, cmd.Flag, exp)
	})

	t.Run("with invalid enum value", func(t *testing.T) {
		cmd := CommandWithFlagWithEnum{}
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

func TestConfigureHandler_variadicArgInput(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("non slice type with variadic arg tag", func(t *testcase.T) {
		p := assert.Panic(t, func() {
			cli.ConfigureHandler(&NonSliceTypeWithVariadicArgTag{}, &cli.Request{
				Args: []string{"foo", "bar", "baz"},
			})
		}, "it is expected that an implementation error results in a panic, as this is non-recoverable at runtime")

		assert.Contains(t, fmt.Sprintf("%v", p), `variadic arg tag selector with ":" only supported for []T types`)
	})
}
