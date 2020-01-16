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

	subject := func(t *testcase.T) error {
		return iterators.Collect(
			t.I(`iterator`).(iterators.Iterator),
			t.I(`slice ptr`),
		)
	}

	s.Let(`slice ptr`, func(t *testcase.T) interface{} {
		return &[]interface{}{}
	})

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		s.Let(`iterator`, func(t *testcase.T) interface{} {
			return iterators.NewEmpty()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			require.Nil(t, subject(t))

			require.Equal(t, 0, reflect.ValueOf(t.I(`slice ptr`)).Elem().Len())
		})
	})

	s.When(`iterator values are primitive type`, func(s *testcase.Spec) {
		s.Let(`iterator`, func(t *testcase.T) interface{} {
			return iterators.NewSlice([]int{1, 2, 3, 4, 5})
		})

		s.And(`the destination slice is an interface`, func(s *testcase.Spec) {
			s.Let(`slice ptr`, func(t *testcase.T) interface{} {
				return &[]interface{}{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []interface{}{1, 2, 3, 4, 5},
					*t.I(`slice ptr`).(*[]interface{}))
			})
		})

		s.And(`the destination slice is has matching element type`, func(s *testcase.Spec) {
			s.Let(`slice ptr`, func(t *testcase.T) interface{} {
				return &[]int{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []int{1, 2, 3, 4, 5},
					*t.I(`slice ptr`).(*[]int))
			})
		})
	})

	s.When(`iterator values are struct type`, func(s *testcase.Spec) {
		type T struct{ V int }

		s.Let(`iterator`, func(t *testcase.T) interface{} {
			return iterators.NewSlice([]T{{V: 1}, {V: 2}, {V: 3}})
		})

		s.And(`the destination slice is an interface`, func(s *testcase.Spec) {
			s.Let(`slice ptr`, func(t *testcase.T) interface{} {
				return &[]interface{}{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []interface{}{T{V: 1}, T{V: 2}, T{V: 3}},
					*t.I(`slice ptr`).(*[]interface{}))
			})
		})

		s.And(`the destination slice is has matching element type`, func(s *testcase.Spec) {
			s.Let(`slice ptr`, func(t *testcase.T) interface{} {
				return &[]T{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []T{{V: 1}, {V: 2}, {V: 3}},
					*t.I(`slice ptr`).(*[]T))
			})
		})
	})

	s.When(`iterator values are pointer type`, func(s *testcase.Spec) {
		type T struct{ V int }

		s.Let(`iterator`, func(t *testcase.T) interface{} {
			return iterators.NewSlice([]*T{{V: 1}, {V: 2}, {V: 3}})
		})

		s.And(`the destination slice is an interface`, func(s *testcase.Spec) {
			s.Let(`slice ptr`, func(t *testcase.T) interface{} {
				return &[]interface{}{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []interface{}{&T{V: 1}, &T{V: 2}, &T{V: 3}},
					*t.I(`slice ptr`).(*[]interface{}))
			})
		})

		s.And(`the destination slice is has matching element type`, func(s *testcase.Spec) {
			s.Let(`slice ptr`, func(t *testcase.T) interface{} {
				return &[]*T{}
			})

			s.Then(`all value fetched from the iterator`, func(t *testcase.T) {
				require.Nil(t, subject(t))
				require.ElementsMatch(t, []*T{{V: 1}, {V: 2}, {V: 3}},
					*t.I(`slice ptr`).(*[]*T))
			})
		})
	})

	s.Describe(`iterator returns error during`, func(s *testcase.Spec) {
		const expectedError = "boom in decode"

		s.Context(`Decode`, func(s *testcase.Spec) {
			s.Let(`iterator`, func(t *testcase.T) interface{} {
				i := iterators.NewMock(iterators.NewSlice([]int{42, 43, 44}))
				i.StubDecode = func(interface{}) error { return errors.New(expectedError) }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				require.EqualError(t, subject(t), expectedError)
			})
		})

		s.Context(`Close`, func(s *testcase.Spec) {
			s.Let(`iterator`, func(t *testcase.T) interface{} {
				i := iterators.NewMock(iterators.NewSlice([]int{42, 43, 44}))
				i.StubClose = func() error { return errors.New(expectedError) }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				require.EqualError(t, subject(t), expectedError)
			})
		})

		s.Context(`Err`, func(s *testcase.Spec) {
			s.Let(`iterator`, func(t *testcase.T) interface{} {
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
