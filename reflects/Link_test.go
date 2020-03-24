package reflects_test

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/reflects"
)

type Example struct {
	Name string
}

var RandomName = fixtures.Random.String()

func ExampleLink() {
	var src = Example{Name: RandomName}
	var dest Example

	if err := reflects.Link(src, &dest); err != nil {
		// handle err
	}
}

func TestLink(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	subject := func(t *testcase.T) error {
		return reflects.Link(t.I(`src`), t.I(`ptr`))
	}

	andPtrPointsToAEmptyInterface := func(s *testcase.Spec) {
		s.And(`ptr points to an empty interface type`, func(s *testcase.Spec) {
			s.Let(`ptr`, func(t *testcase.T) interface{} {
				var i interface{}
				return &i
			})

			s.Then(`it will link the value`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.Equal(t, t.I(`src`), *t.I(`ptr`).(*interface{}))
			})
		})
	}

	andPtrPointsToSomethingWithTheSameType := func(s *testcase.Spec, ptrValue func() interface{}) {
		s.And(`ptr is pointing to the same type`, func(s *testcase.Spec) {
			s.Let(`ptr`, func(t *testcase.T) interface{} {
				return ptrValue()
			})

			s.Then(`ptr pointed value equal with source value`, func(t *testcase.T) {
				require.Nil(t, subject(t))

				require.Equal(t, t.I(`src`), reflect.ValueOf(t.I(`ptr`)).Elem().Interface())
			})
		})
	}

	s.When(`to be linked value is`, func(s *testcase.Spec) {
		s.Context(`a primitive non pointer type`, func(s *testcase.Spec) {
			s.Let(`src`, func(t *testcase.T) interface{} {
				return `Hello, World!`
			})

			andPtrPointsToAEmptyInterface(s)
			andPtrPointsToSomethingWithTheSameType(s, func() interface{} {
				var s string
				return &s
			})
		})

		type T struct{ str string }

		s.Context(`a struct type`, func(s *testcase.Spec) {
			s.Let(`src`, func(t *testcase.T) interface{} {
				return T{str: RandomName}
			})

			andPtrPointsToAEmptyInterface(s)
			andPtrPointsToSomethingWithTheSameType(s, func() interface{} {
				return &T{}
			})
		})

		s.Context(`a pointer to a struct type`, func(s *testcase.Spec) {
			s.Let(`src`, func(t *testcase.T) interface{} {
				return &T{str: RandomName}
			})

			andPtrPointsToAEmptyInterface(s)
			andPtrPointsToSomethingWithTheSameType(s, func() interface{} {
				value := &T{}
				return &value
			})
		})
	})
}
