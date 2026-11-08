package cli_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

const (
	vMinEnvKey    = "CLI_TEST_V_MIN"    // the env tag of VRange.Min
	vMaxEnvKey    = "CLI_TEST_V_MAX"    // the env tag of VRange.Max
	vFormatEnvKey = "CLI_TEST_V_FORMAT" // the env tag of VOptions.Format
)

type VTrace struct{ Calls []string }

type vTraceKey struct{}

// vTraceCall records the call into the VTrace of the context, if it has one.
// It lets a Validatable input, which can't hold a VTrace, tell when it was validated.
func vTraceCall(ctx context.Context, call string) {
	if tr, ok := ctx.Value(vTraceKey{}).(*VTrace); ok {
		tr.Calls = append(tr.Calls, call)
	}
}

// VRange is a Validatable with a value receiver, whose inputs are only valid together.
type VRange struct {
	Trace *VTrace `opaque:"true"`
	Min   int     `env:"CLI_TEST_V_MIN" default:"1"`
	Max   int     `env:"CLI_TEST_V_MAX" default:"10"`
}

func (r VRange) Validate(context.Context) error {
	r.Trace.Calls = append(r.Trace.Calls, "range")
	if r.Max < r.Min {
		return fmt.Errorf("min (%d) can't be greater than max (%d)", r.Min, r.Max)
	}
	return nil
}

// VDependency is a Validatable with a pointer receiver, which holds a Validatable field as well.
type VDependency struct {
	Trace *VTrace `opaque:"true"`
	Range VRange
	Err   error
}

func (d *VDependency) Validate(context.Context) error {
	d.Trace.Calls = append(d.Trace.Calls, "dependency")
	return d.Err
}

// VLevel is a Validatable input, which is not valid with its zero value.
type VLevel string

func (l VLevel) Validate(ctx context.Context) error {
	vTraceCall(ctx, "level")
	if l == "" {
		return errors.New("level can't be empty")
	}
	return nil
}

// VOptions is a Validatable struct input with a pointer receiver, which is not valid with its zero value.
// It gets its value as a JSON object, or through its inner inputs.
type VOptions struct {
	Format string `env:"CLI_TEST_V_FORMAT" optional:"true"`
	Style  VStyle
}

// VStyle is a Validatable with a value receiver, which is not an input itself, but a value of a struct input.
type VStyle string

func (s VStyle) Validate(ctx context.Context) error {
	vTraceCall(ctx, "style")
	return nil
}

func (o *VOptions) Validate(context.Context) error {
	switch o.Format {
	case "json", "text":
		return nil
	default:
		return fmt.Errorf("unknown format: %q", o.Format)
	}
}

// VAudit is a Validatable with a value receiver, which tells by its Label when it was validated.
type VAudit struct {
	Trace *VTrace `opaque:"true"`
	Label string
}

func (a VAudit) Validate(context.Context) error {
	a.Trace.Calls = append(a.Trace.Calls, a.Label)
	return nil
}

// VAuditing is a Validatable through the Validate method it promotes from the Validatable it embeds.
type VAuditing struct{ VAudit }

// VCommand embeds a Validatable, while it has a Validate method of its own, which shadows the embedded one.
type VCommand struct {
	VAudit
	Dependency VDependency
	Auditing   VAuditing
	Level      VLevel   `flag:"level"`
	Options    VOptions `flag:"options"`
	Trace      *VTrace  `opaque:"true"`
	Err        error
}

func (cmd VCommand) Validate(context.Context) error {
	cmd.Trace.Calls = append(cmd.Trace.Calls, "handler")
	return cmd.Err
}

func (cmd VCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	cmd.Trace.Calls = append(cmd.Trace.Calls, "cmd")
}

func TestValidatable(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		trace = let.Var(s, func(t *testcase.T) *VTrace {
			return &VTrace{}
		})
		dependencyErr = let.VarOf[error](s, nil)
		handlerErr    = let.VarOf[error](s, nil)
		// minInput is greater than the default of VRange.Max
		minInput = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntBetween(11, 100)
		})
	)
	handler := let.Var(s, func(t *testcase.T) VCommand {
		return VCommand{
			VAudit:     VAudit{Trace: trace.Get(t), Label: "embedded"},
			Dependency: VDependency{Trace: trace.Get(t), Range: VRange{Trace: trace.Get(t)}, Err: dependencyErr.Get(t)},
			Auditing:   VAuditing{VAudit: VAudit{Trace: trace.Get(t), Label: "auditing"}},
			Trace:      trace.Get(t),
			Err:        handlerErr.Get(t),
		}
	})
	count := func(t *testcase.T, call string) int {
		return len(slicekit.Filter(trace.Get(t).Calls, func(c string) bool { return c == call }))
	}

	s.Before(func(t *testcase.T) {
		testcase.UnsetEnv(t, vMinEnvKey)
		testcase.UnsetEnv(t, vMaxEnvKey)
		testcase.UnsetEnv(t, vFormatEnvKey)
		t.OnFail(func() { t.Log("calls:", trace.Get(t).Calls) })
	})

	s.Describe("ServeCLI", func(s *testcase.Spec) {
		var (
			response = let.Var(s, func(t *testcase.T) *cli.ResponseRecorder {
				return &cli.ResponseRecorder{}
			})
			args = let.Var(s, func(t *testcase.T) []string {
				return nil
			})
			request = let.Var(s, func(t *testcase.T) *cli.Request {
				r := &cli.Request{Args: args.Get(t)}
				return r.WithContext(context.WithValue(t.Context(), vTraceKey{}, trace.Get(t)))
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

		s.Then("the validatable values are validated, inner first and the handler last, before the command is executed", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.Equal(t, trace.Get(t).Calls, []string{"range", "dependency", "auditing", "handler", "cmd"})
		})

		s.Then("a validatable embedded into a validatable field is validated once, through the field that embeds it", func(t *testcase.T) {
			act(t)

			assert.Equal(t, count(t, "auditing"), 1)
		})

		s.Then("a validatable embedded into the handler is left to the Validate of the handler, which shadows it, like in Go", func(t *testcase.T) {
			act(t)

			assert.Equal(t, count(t, "embedded"), 0)
		})

		s.Then("the optional inputs which were not given are not validated for their zero value", func(t *testcase.T) {
			act(t)

			assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
			assert.Contains(t, trace.Get(t).Calls, "cmd")
		})

		s.When("the inputs of a validatable value are not valid together", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, vMinEnvKey, strconv.Itoa(minInput.Get(t)))
			})

			s.Then("the command is not executed", func(t *testcase.T) {
				act(t)

				assert.NotContains(t, trace.Get(t).Calls, "cmd")
			})

			s.Then("it replies with a bad request, the usage and the validation error", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "Usage:")
				assert.Contains(t, response.Get(t).Err.String(),
					fmt.Sprintf("min (%d) can't be greater than max (10)", minInput.Get(t)))
			})
		})

		s.When("a dependency is not valid", func(s *testcase.Spec) {
			dependencyErr.Let(s, func(t *testcase.T) error {
				return t.Random.Error()
			})

			s.Then("the command is not executed", func(t *testcase.T) {
				act(t)

				assert.NotContains(t, trace.Get(t).Calls, "cmd")
			})

			s.Then("it replies with a bad request and the validation error", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), dependencyErr.Get(t).Error())
			})
		})

		s.When("the handler is not valid", func(s *testcase.Spec) {
			handlerErr.Let(s, func(t *testcase.T) error {
				return t.Random.Error()
			})

			s.Then("the command is not executed", func(t *testcase.T) {
				act(t)

				assert.NotContains(t, trace.Get(t).Calls, "cmd")
			})

			s.Then("it replies with a bad request and the validation error", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), handlerErr.Get(t).Error())
			})
		})

		s.When("a validatable input is given", func(s *testcase.Spec) {
			level := let.StringNC(s, 5, random.CharsetAlpha())

			args.Let(s, func(t *testcase.T) []string {
				return []string{"-level=" + level.Get(t)}
			})

			s.Then("the input is validated once", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.Equal(t, count(t, "level"), 1)
			})
		})

		s.When("a validatable input is given with a value which is not valid", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-level="}
			})

			s.Then("it replies with a bad request and the validation error of the input", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), "level can't be empty")
				assert.NotContains(t, trace.Get(t).Calls, "cmd")
			})
		})

		s.When("a struct input gets a value through its inner inputs", func(s *testcase.Spec) {
			format := let.StringNC(s, 8, random.CharsetAlpha())

			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, vFormatEnvKey, format.Get(t))
			})

			s.Then("the struct input is validated as a whole", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), fmt.Sprintf("unknown format: %q", format.Get(t)))
				assert.NotContains(t, trace.Get(t).Calls, "cmd")
			})
		})

		s.When("a struct input is given with a validatable inner value", func(s *testcase.Spec) {
			style := let.StringNC(s, 5, random.CharsetAlpha())

			args.Let(s, func(t *testcase.T) []string {
				return []string{fmt.Sprintf(`-options={"Format":"json","Style":%q}`, style.Get(t))}
			})

			s.Then("the inner value is validated once", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeOK)
				assert.Equal(t, count(t, "style"), 1)
			})
		})

		s.When("a struct input is given with a value which is not valid", func(s *testcase.Spec) {
			format := let.StringNC(s, 8, random.CharsetAlpha())

			args.Let(s, func(t *testcase.T) []string {
				return []string{fmt.Sprintf(`-options={"Format":%q}`, format.Get(t))}
			})

			s.Then("it replies with a bad request and the validation error of the input", func(t *testcase.T) {
				act(t)

				assert.Equal(t, response.Get(t).Code, cli.ExitCodeBadRequest)
				assert.Contains(t, response.Get(t).Err.String(), fmt.Sprintf("unknown format: %q", format.Get(t)))
				assert.NotContains(t, trace.Get(t).Calls, "cmd")
			})
		})
	})

	s.Describe("Build", func(s *testcase.Spec) {
		var (
			ptr = let.Var(s, func(t *testcase.T) *VCommand {
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

		s.Then("the validatable values are validated, inner first and the handler last, without executing the command", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, trace.Get(t).Calls, []string{"range", "dependency", "auditing", "handler"})
		})

		s.Then("a validatable embedded into a validatable field is validated once, through the field that embeds it", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, count(t, "auditing"), 1)
		})

		s.Then("a validatable embedded into the handler is left to the Validate of the handler, which shadows it, like in Go", func(t *testcase.T) {
			_, err := act(t)
			assert.NoError(t, err)

			assert.Equal(t, count(t, "embedded"), 0)
		})

		s.When("the inputs of a validatable value are not valid together", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				testcase.SetEnv(t, vMinEnvKey, strconv.Itoa(minInput.Get(t)))
			})

			s.Then("it returns an invalid input error with the validation error", func(t *testcase.T) {
				_, err := act(t)

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				verr, ok := eii.ValidateError()
				assert.True(t, ok)
				assert.Contains(t, verr.Cause.Error(),
					fmt.Sprintf("min (%d) can't be greater than max (10)", minInput.Get(t)))
			})
		})

		s.When("a dependency is not valid", func(s *testcase.Spec) {
			dependencyErr.Let(s, func(t *testcase.T) error {
				return t.Random.Error()
			})

			s.Then("it returns an invalid input error, which wraps the error of the dependency", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, dependencyErr.Get(t))

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				verr, ok := eii.ValidateError()
				assert.True(t, ok)
				assert.Equal(t, verr.Cause, dependencyErr.Get(t))
			})

			s.Then("the error message names the dependency field, and tells the cause", func(t *testcase.T) {
				_, err := act(t)

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				assert.Equal(t, err.Error(), "[ErrInvalidInput] cli_test.VCommand.Dependency: "+eii.Err.Error())
			})
		})

		s.When("the handler is not valid", func(s *testcase.Spec) {
			handlerErr.Let(s, func(t *testcase.T) error {
				return t.Random.Error()
			})

			s.Then("it returns an invalid input error, which wraps the error of the handler", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, handlerErr.Get(t))

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				verr, ok := eii.ValidateError()
				assert.True(t, ok)
				assert.Equal(t, verr.Cause, handlerErr.Get(t))
			})

			s.Then("the error message tells the cause", func(t *testcase.T) {
				_, err := act(t)

				var eii cli.ErrInvalidInput
				assert.True(t, errors.As(err, &eii))
				assert.Equal(t, err.Error(), "[ErrInvalidInput]: "+eii.Err.Error())
			})
		})
	})
}
