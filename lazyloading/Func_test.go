package lazyloading_test

import (
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/lazyloading"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func ExampleFunc() {
	value := lazyloading.Func(func() interface{} {
		// my expensive calculation's result
		return 42
	})

	// one eternity later
	fmt.Println(value().(int))
}

func TestFunc(t *testing.T) {
	s := testcase.NewSpec(t)

	initWasCalled := s.LetValue(`init was called`, false)
	init := s.Let(`init block`, func(t *testcase.T) interface{} {
		initWasCalled.Set(t, true)
		return func() interface{} { return t.Random.Int() }
	})
	initGet := func(t *testcase.T) func() interface{} {
		return init.Get(t).(func() interface{})
	}
	subject := func(t *testcase.T) func() interface{} {
		return lazyloading.Func(initGet(t))
	}

	s.Test(`assuming that init block always yield a different value`, func(t *testcase.T) {
		require.NotEqual(t, initGet(t)(), initGet(t)())
	})

	s.Then(`calling lazy loaded value multiple times return the same result`, func(t *testcase.T) {
		llv := subject(t)
		require.Equal(t, llv(), llv())
	})

	s.Then(`before calling lazy loaded value, init block is not used`, func(t *testcase.T) {
		require.False(t, initWasCalled.Get(t).(bool))
		subject(t)()
		require.True(t, initWasCalled.Get(t).(bool))
	})

	s.When(`wrapped value is a pointer type`, func(s *testcase.Spec) {
		init.Let(s, func(t *testcase.T) interface{} {
			var n int = 42
			return func() interface{} { return &n }
		})

		s.Then(`lazy loaded value resolver always return the same object`, func(t *testcase.T) {
			llv := subject(t)
			expected := t.Random.Int()
			*llv().(*int) = expected
			require.Equal(t, expected, *llv().(*int))
		})
	})

	s.Test(`safe for concurrent use`, func(t *testcase.T) {
		llv := subject(t)

		testcase.Race(func() { llv() }, func() { llv() })
	})
}
