package iterators_test

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/ports/iterators"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestCollect(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		iterator = testcase.Var[iterators.Iterator[int]]{ID: `iterator`}
		subject  = func(t *testcase.T) ([]int, error) {
			return iterators.Collect(iterator.Get(t))
		}
	)

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iterators.Iterator[int] {
			return iterators.Empty[int]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs, err := subject(t)
			t.Must.Nil(err)
			t.Must.Empty(vs)
		})
	})

	s.When(`iterator has elements`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iterators.Iterator[int] {
			return iterators.Slice([]int{1, 2, 3})
		})

		s.Then(`it will collect the values`, func(t *testcase.T) {
			vs, err := subject(t)
			t.Must.Nil(err)
			t.Must.Equal([]int{1, 2, 3}, vs)
		})
	})

	s.Describe(`iterator returns error during`, func(s *testcase.Spec) {
		const expectedErr errorkit.Error = "boom"

		s.Context(`Close`, func(s *testcase.Spec) {
			iterator.Let(s, func(t *testcase.T) iterators.Iterator[int] {
				i := iterators.Stub[int](iterators.Slice([]int{42, 43, 44}))
				i.StubClose = func() error { return expectedErr }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				_, err := subject(t)
				t.Must.Equal(err, expectedErr)
			})
		})

		s.Context(`Err`, func(s *testcase.Spec) {
			iterator.Let(s, func(t *testcase.T) iterators.Iterator[int] {
				i := iterators.Stub[int](iterators.Slice([]int{42, 43, 44}))
				i.StubErr = func() error { return expectedErr }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				_, err := subject(t)
				t.Must.Equal(err, expectedErr)
			})
		})
	})
}

func TestCollect_emptySlice(t *testing.T) {
	T := 0
	slice := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(T)), 0, 0).Interface()
	t.Logf(`%T`, slice)
	t.Logf(`%#v`, slice)
	vs, err := iterators.Collect[int](iterators.Slice[int]([]int{42}))
	assert.Must(t).Nil(err)
	assert.Must(t).Equal([]int{42}, vs)
}
