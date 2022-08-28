package reflects_test

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/reflects"
	"math/rand"
	"reflect"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type Example struct {
	Name string
}

var RandomName = fmt.Sprintf("%d", rand.Int())

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

	var (
		src = testcase.Var[any]{ID: "src"}
		ptr = testcase.Var[any]{ID: "ptr"}
	)
	subject := func(t *testcase.T) error {
		return reflects.Link(src.Get(t), ptr.Get(t))
	}

	andPtrPointsToAEmptyInterface := func(s *testcase.Spec) {
		s.And(`ptr points to an empty interface type`, func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				var i interface{}
				return &i
			})

			s.Then(`it will link the value`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))
				assert.Must(t).Equal(src.Get(t), *ptr.Get(t).(*any))
			})
		})
	}

	andPtrPointsToSomethingWithTheSameType := func(s *testcase.Spec, ptrValue func() interface{}) {
		s.And(`ptr is pointing to the same type`, func(s *testcase.Spec) {
			ptr.Let(s, func(t *testcase.T) any {
				return ptrValue()
			})

			s.Then(`ptr pointed value equal with source value`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				assert.Must(t).Equal(src.Get(t), reflect.ValueOf(ptr.Get(t)).Elem().Interface())
			})
		})
	}

	s.When(`to be linked value is`, func(s *testcase.Spec) {
		s.Context(`a primitive non pointer type`, func(s *testcase.Spec) {
			src.Let(s, func(t *testcase.T) interface{} {
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
			src.Let(s, func(t *testcase.T) interface{} {
				return T{str: RandomName}
			})

			andPtrPointsToAEmptyInterface(s)
			andPtrPointsToSomethingWithTheSameType(s, func() interface{} {
				return &T{}
			})
		})

		s.Context(`a pointer to a struct type`, func(s *testcase.Spec) {
			src.Let(s, func(t *testcase.T) interface{} {
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
