package iterators_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

func TestCollect(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	var (
		iterator = testcase.Var{Name: `iterator`}
		slicePtr = s.Let(`slice pointer`, func(t *testcase.T) interface{} {
			return &[]interface{}{}
		})
		subject = func(t *testcase.T) error {
			return iterators.Collect(
				iterator.Get(t).(iterators.Interface),
				slicePtr.Get(t),
			)
		}
	)

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) interface{} {
			return iterators.NewEmpty()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			require.Nil(t, subject(t))
			require.Len(t, *slicePtr.Get(t).(*[]interface{}), 0)
		})
	})

	s.When(`iterator values are primitive type`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) interface{} {
			return iterators.NewSlice([]int{1, 2, 3, 4, 5})
		})

		s.And(`the destination slice is an interface`, func(s *testcase.Spec) {
			slicePtr.Let(s, func(t *testcase.T) interface{} {
				return &[]interface{}{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []interface{}{1, 2, 3, 4, 5},
					*slicePtr.Get(t).(*[]interface{}))
			})
		})

		s.And(`the destination slice is has matching element type`, func(s *testcase.Spec) {
			slicePtr.Let(s, func(t *testcase.T) interface{} {
				return &[]int{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []int{1, 2, 3, 4, 5},
					*slicePtr.Get(t).(*[]int))
			})
		})
	})

	s.When(`iterator values are struct type`, func(s *testcase.Spec) {
		type T struct{ V int }

		iterator.Let(s, func(t *testcase.T) interface{} {
			return iterators.NewSlice([]T{{V: 1}, {V: 2}, {V: 3}})
		})

		s.And(`the destination slice is an interface`, func(s *testcase.Spec) {
			slicePtr.Let(s, func(t *testcase.T) interface{} {
				return &[]interface{}{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []interface{}{T{V: 1}, T{V: 2}, T{V: 3}},
					*slicePtr.Get(t).(*[]interface{}))
			})
		})

		s.And(`the destination slice is has matching element type`, func(s *testcase.Spec) {
			slicePtr.Let(s, func(t *testcase.T) interface{} {
				return &[]T{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []T{{V: 1}, {V: 2}, {V: 3}},
					*slicePtr.Get(t).(*[]T))
			})
		})
	})

	s.When(`iterator values are pointer type`, func(s *testcase.Spec) {
		type T struct{ V int }

		iterator.Let(s, func(t *testcase.T) interface{} {
			return iterators.NewSlice([]*T{{V: 1}, {V: 2}, {V: 3}})
		})

		s.And(`the destination slice is an interface`, func(s *testcase.Spec) {
			slicePtr.Let(s, func(t *testcase.T) interface{} {
				return &[]interface{}{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []interface{}{&T{V: 1}, &T{V: 2}, &T{V: 3}},
					*slicePtr.Get(t).(*[]interface{}))
			})
		})

		s.And(`the destination slice is has matching element type`, func(s *testcase.Spec) {
			slicePtr.Let(s, func(t *testcase.T) interface{} {
				return &[]*T{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []*T{{V: 1}, {V: 2}, {V: 3}},
					*slicePtr.Get(t).(*[]*T))
			})
		})
	})

	s.Describe(`iterator returns error during`, func(s *testcase.Spec) {
		const expectedError = "boom in decode"

		s.Context(`Decode`, func(s *testcase.Spec) {
			iterator.Let(s, func(t *testcase.T) interface{} {
				i := iterators.NewMock(iterators.NewSlice([]int{42, 43, 44}))
				i.StubDecode = func(interface{}) error { return errors.New(expectedError) }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				require.EqualError(t, subject(t), expectedError)
			})
		})

		s.Context(`Close`, func(s *testcase.Spec) {
			iterator.Let(s, func(t *testcase.T) interface{} {
				i := iterators.NewMock(iterators.NewSlice([]int{42, 43, 44}))
				i.StubClose = func() error { return errors.New(expectedError) }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				require.EqualError(t, subject(t), expectedError)
			})
		})

		s.Context(`Err`, func(s *testcase.Spec) {
			iterator.Let(s, func(t *testcase.T) interface{} {
				i := iterators.NewMock(iterators.NewSlice([]int{42, 43, 44}))
				i.StubErr = func() error { return errors.New(expectedError) }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				require.EqualError(t, subject(t), expectedError)
			})
		})
	})
}

func TestCollect_emptySlice(t *testing.T) {
	T := 0
	slice := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(T)), 0, 0).Interface()
	t.Logf(`%T`, slice)
	t.Logf(`%#v`, slice)
	require.Nil(t, iterators.Collect(iterators.NewSlice([]int{42}), &slice))
	require.Equal(t, []int{42}, slice)
}
