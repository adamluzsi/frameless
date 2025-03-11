package errorkit_test

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/runtimekit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func ExampleWithTrace() {
	const ErrBase errorkit.Error = "some error that we get back from a function call"

	// traced error that we can return
	err := errorkit.WithTrace(ErrBase)
	// if we receive back a traced error from a function call, calling WithTrace wonn't reset the trace
	err = errorkit.WithTrace(err)
	// maybe we have some additional trace wrapping
	err = fmt.Errorf("wrapping the erro: %w", err)

	var traced errorkit.TracedError
	_ = errors.As(err, &traced) // true
	_ = traced.Stack            // stack trace
}

func TestWithTrace(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		inputErr = testcase.Let[error](s, nil)
	)
	act := func(t *testcase.T) error {
		return errorkit.WithTrace(inputErr.Get(t))
	}

	s.When("input error is nil", func(s *testcase.Spec) {
		inputErr.LetValue(s, nil)

		s.Then("the returned error is also nil", func(t *testcase.T) {
			assert.Nil(t, act(t))
		})
	})

	s.When("input error doesn't have trace", func(s *testcase.Spec) {
		inputErr.Let(s, func(t *testcase.T) error {
			return t.Random.Error()
		})

		s.Then("wrapped input error can be asserted with errors.Is", func(t *testcase.T) {
			got := act(t)

			assert.NotNil(t, got)
			assert.True(t, errors.Is(got, inputErr.Get(t)))
		})

		s.And("the input error is a named error", func(s *testcase.Spec) {
			inputErr.Let(s, func(t *testcase.T) error {
				return ErrT{V: t.Random.String()}
			})

			s.Then("it can be retrieved with the errors.As", func(t *testcase.T) {
				got := act(t)

				assert.NotNil(t, got)

				var errGot ErrT
				assert.True(t, errors.As(got, &errGot))
				assert.Equal(t, errGot, inputErr.Get(t).(ErrT))
			})
		})

		s.Then("the returned error will have trace in its .Error()", func(t *testcase.T) {
			got := act(t)

			assert.NotNil(t, got)
			assert.Contain(t, got.Error(), "trace_test.go")
		})

		s.Then("the trace doesn't contain the errorkit package", func(t *testcase.T) {
			got := act(t)

			assert.NotNil(t, got)
			assert.NotContain(t, got.Error(), "pkg/errorkit.")
		})

		s.Then("the trace doesn't contain some standard lib that would just make it difficult to read the trace", func(t *testcase.T) {
			got := act(t)

			assert.NotNil(t, got)
			assert.NotContain(t, got.Error(), "runtime")
			assert.NotContain(t, got.Error(), "testing")
		})

		s.Then("trace can be accessed directly with errors.As", func(t *testcase.T) {
			got := act(t)
			assert.NotNil(t, got)

			var traced errorkit.TracedError
			assert.True(t, errors.As(got, &traced))
			assert.NotNil(t, traced.Err)
			assert.NotEmpty(t, traced.Stack)
		})
	})

	s.When("input error alreay has a trace", func(s *testcase.Spec) {
		tracedErr := testcase.Let(s, func(t *testcase.T) errorkit.TracedError {
			return errorkit.TracedError{Err: t.Random.Error(), Stack: []runtime.Frame{{Function: "foo"}}}
		})

		inputErr.Let(s, func(t *testcase.T) error {
			t.Log("given the traced error idiomatically wrapped with further details")
			return fmt.Errorf("traced error: %w", tracedErr.Get(t))
		})

		s.Then("the original trace is honoured", func(t *testcase.T) {
			t.Log("this behaviour enables the use of WithTrace without the fear of overwriting the underlying existing trace value")

			got := act(t)
			assert.NotNil(t, got)

			var traced errorkit.TracedError
			assert.True(t, errors.As(got, &traced))
			assert.Equal(t, traced, tracedErr.Get(t))

			assert.Equal(t, got.Error(), inputErr.Get(t).Error(),
				"since nothing is changed, the two error should have the same output")
		})
	})
}

func TestRegisterTraceException_smoke(t *testing.T) {
	err := errorkit.WithTrace(errorkit.Error("boom"))
	assert.Error(t, err)
	assert.Contain(t, err.Error(), t.Name())

	undo := runtimekit.RegisterTraceException(func(f runtime.Frame) bool {
		return strings.Contains(f.Function, t.Name())
	})

	err = errorkit.WithTrace(errorkit.Error("boom"))
	assert.Error(t, err)
	assert.NotContain(t, err.Error(), t.Name())

	undo()

	err = errorkit.WithTrace(errorkit.Error("boom"))
	assert.Error(t, err)
	assert.Contain(t, err.Error(), t.Name())
}
