package lazyloading_test

import (
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/pkg/lazyloading"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
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
	init := testcase.Let(s, func(t *testcase.T) interface{} {
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
		t.Must.NotEqual(initGet(t)(), initGet(t)())
	})

	s.Then(`calling lazy loaded value multiple times return the same result`, func(t *testcase.T) {
		llv := subject(t)
		t.Must.Equal(llv(), llv())
	})

	s.Then(`before calling lazy loaded value, init block is not used`, func(t *testcase.T) {
		assert.Must(t).False(initWasCalled.Get(t).(bool))
		subject(t)()
		assert.Must(t).True(initWasCalled.Get(t).(bool))
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
			assert.Must(t).Equal(expected, *llv().(*int))
		})
	})

	s.Test(`safe for concurrent use`, func(t *testcase.T) {
		llv := subject(t)

		testcase.Race(func() { llv() }, func() { llv() })
	})
}
