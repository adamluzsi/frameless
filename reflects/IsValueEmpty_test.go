package reflects_test

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func TestIsValueEmpty(t *testing.T) {
	s := testcase.NewSpec(t)
	val := testcase.Var[any]{ID: `input value`}
	subject := func(t *testcase.T) bool {
		return reflects.IsValueEmpty(reflect.ValueOf(val.Get(t)))
	}

	s.When(`value is an nil pointer`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var ptr *string
			return ptr
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an pointer to an zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := ""
			return &v
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an pointer to non zero value`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			v := "Hello, world!"
			return &v
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var slice []string
			return slice
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an empty slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{}
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an populated slice`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return []string{"foo", "bar", "baz"}
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var m map[string]struct{}
			return m
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an empty map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{}
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an populated map`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return map[string]struct{}{
				"foo": {},
				"bar": {},
				"baz": {},
			}
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized chan`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var m chan struct{}
			return m
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an initialized chan`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return make(chan struct{})
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})

	s.When(`value is an uninitialized func`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			var fn func()
			return fn
		})

		s.Then(`it will be reported as empty`, func(t *testcase.T) {
			assert.Must(t).True(subject(t))
		})
	})

	s.When(`value is an initialized func`, func(s *testcase.Spec) {
		val.Let(s, func(t *testcase.T) interface{} {
			return func() {}
		})

		s.Then(`it will be reported as non-empty`, func(t *testcase.T) {
			assert.Must(t).False(subject(t))
		})
	})
}
