package errorkit_test

import (
	"errors"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/testcase/assert"
)

func ExampleError_Error() {
	const ErrSomething errorkit.Error = "something is an error"

	_ = ErrSomething
}

func TestError_Error_smoke(t *testing.T) {
	const ErrExample errorkit.Error = "ErrExample"
	assert.Equal(t, ErrExample.Error(), string(ErrExample))
}

type ErrAsStub struct {
	V string
}

func (err ErrAsStub) Error() string {
	return fmt.Sprintf("ErrAsStub: %s", err.V)
}

func TestError_Wrap_smoke(t *testing.T) {
	const ErrExample errorkit.Error = "ErrExample"
	t.Run("happy", func(t *testing.T) {
		exp := rnd.Error()
		got := ErrExample.Wrap(exp)
		assert.ErrorIs(t, got, exp)
		assert.ErrorIs(t, got, ErrExample)
		assert.Contain(t, got.Error(), fmt.Sprintf("[%s] %s", ErrExample, exp.Error()))

		t.Run("Is", func(t *testing.T) {
			assert.True(t, errors.Is(got, ErrExample))
			assert.True(t, errors.Is(got, exp))
		})

		t.Run("As", func(t *testing.T) {
			exp := ErrAsStub{V: rnd.String()}
			got := ErrExample.Wrap(exp)
			assert.ErrorIs(t, got, exp)
			assert.ErrorIs(t, got, ErrExample)

			var expected ErrAsStub
			assert.True(t, errors.As(got, &expected))
			assert.Equal(t, exp, expected)
		})
	})
	t.Run("nil", func(t *testing.T) {
		got := ErrExample.Wrap(nil)
		assert.ErrorIs(t, got, ErrExample)
		assert.Equal[error](t, got, ErrExample)
	})
}

func TestError_F_smoke(t *testing.T) {
	const ErrExample errorkit.Error = "ErrExample"
	t.Run("sprintf", func(t *testing.T) {
		got := ErrExample.F("foo - bar - %s", "baz")
		assert.ErrorIs(t, got, ErrExample)
		assert.Contain(t, got.Error(), "foo - bar - baz")
	})
	t.Run("errorf", func(t *testing.T) {
		exp := rnd.Error()
		got := ErrExample.F("%w", exp)
		assert.ErrorIs(t, got, ErrExample)
		assert.ErrorIs(t, got, exp)
		assert.Contain(t, got.Error(), ErrExample.Error())
	})
}
