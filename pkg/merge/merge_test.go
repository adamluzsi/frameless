package merge_test

import (
	"errors"
	"go.llib.dev/frameless/pkg/merge"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"testing"
)

func ExampleSlice() {
	var (
		a       = []int{1, 2, 3}
		b       = []int{7, 8, 9}
		c       = []int{4, 5, 6}
		d []int = nil
	)
	got := merge.Slice(a, b, c, d)
	_ = got // []int{1, 2, 3, 7, 8, 9, 4, 5, 6}
}

func TestSlice(t *testing.T) {
	var (
		a       = []int{1, 2, 3}
		b       = []int{7, 8, 9}
		c       = []int{4, 5, 6}
		d []int = nil
	)
	got := merge.Slice(a, b, c, d)
	assert.Equal(t, got, []int{1, 2, 3, 7, 8, 9, 4, 5, 6})
}

func ExampleMap() {
	var (
		a = map[string]int{"a": 1, "b": 2, "c": 3}
		b = map[string]int{"g": 7, "h": 8, "i": 9}
		c = map[string]int{"d": 4, "e": 5, "f": 6}
		d = map[string]int{"a": 42}
	)
	got := merge.Map(a, b, c, d)
	_ = got
	//
	//	map[string]int{
	//		"a": 42, "b": 2, "c": 3,
	//		"g": 7, "h": 8, "i": 9,
	//		"d": 4, "e": 5, "f": 6,
	//	}
}

func TestMap(t *testing.T) {
	var (
		a = map[string]int{"a": 1, "b": 2, "c": 3}
		b = map[string]int{"g": 7, "h": 8, "i": 9}
		c = map[string]int{"d": 4, "e": 5, "f": 6}
		d = map[string]int{"a": 42}
	)
	got := merge.Map(a, b, c, d)
	assert.Equal(t, got, map[string]int{
		"b": 2, "c": 3,
		"g": 7, "h": 8, "i": 9,
		"d": 4, "e": 5, "f": 6,
		"a": 42,
	})
}

func ExampleError() {
	var (
		err1 error = errors.New("first error")
		err2 error = errors.New("second error")
		err3 error = nil
	)

	err := merge.Error(err1, err2, err3)
	errors.Is(err, err1) // true
	errors.Is(err, err2) // true
	errors.Is(err, err3) // true
}

type (
	ErrType1 struct{}
	ErrType2 struct{ V int }
)

func (err ErrType1) Error() string { return "ErrType1" }
func (err ErrType2) Error() string { return "ErrType2" }

func TestError(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		errs = testcase.Let[[]error](s, nil)
	)
	act := func(t *testcase.T) error {
		return merge.Error(errs.Get(t)...)
	}

	s.When("no error is supplied", func(s *testcase.Spec) {
		errs.Let(s, func(t *testcase.T) []error {
			return []error{}
		})

		s.Then("it will return with nil", func(t *testcase.T) {
			t.Must.Nil(act(t))
		})

		s.Then("errors.Is yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.Is(err, ErrType1{}))
			t.Must.False(errors.Is(err, ErrType2{}))
		})

		s.Then("errors.As yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.As(err, &ErrType1{}))
			t.Must.False(errors.As(err, &ErrType2{}))
		})
	})

	s.When("an error value is supplied", func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		errs.Let(s, func(t *testcase.T) []error {
			return []error{expectedErr.Get(t)}
		})

		s.Then("the exact value is returned", func(t *testcase.T) {
			t.Must.Equal(expectedErr.Get(t), act(t))
		})

		s.Then("errors.Is yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.Is(err, ErrType1{}))
			t.Must.False(errors.Is(err, ErrType2{}))
		})

		s.Then("errors.As yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.As(err, &ErrType1{}))
			t.Must.False(errors.As(err, &ErrType2{}))
		})

		s.And("the error value is a typed error value", func(s *testcase.Spec) {
			expectedErr.LetValue(s, ErrType1{})

			s.Then("the exact value is returned", func(t *testcase.T) {
				t.Must.Equal(expectedErr.Get(t), act(t))
			})

			s.Then("errors.Is will find wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.Is(err, ErrType1{}))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As will find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.As(err, &ErrType1{}))
				t.Must.False(errors.As(err, &ErrType2{}))
			})
		})

		s.And("but the error value is nil", func(s *testcase.Spec) {
			expectedErr.LetValue(s, nil)

			s.Then("it will return with nil", func(t *testcase.T) {
				t.Must.Nil(act(t))
			})

			s.Then("errors.Is yield false", func(t *testcase.T) {
				err := act(t)
				t.Must.False(errors.Is(err, ErrType1{}))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As yield false", func(t *testcase.T) {
				err := act(t)
				t.Must.False(errors.As(err, &ErrType1{}))
				t.Must.False(errors.As(err, &ErrType2{}))
			})
		})
	})

	s.When("multiple error values are supplied", func(s *testcase.Spec) {
		expectedErr1 := let.Error(s)
		expectedErr2 := let.Error(s)
		expectedErr3 := let.Error(s)

		errs.Let(s, func(t *testcase.T) []error {
			return []error{
				expectedErr1.Get(t),
				expectedErr2.Get(t),
				expectedErr3.Get(t),
			}
		})

		s.Then("retruned value includes all three error value", func(t *testcase.T) {
			err := act(t)
			t.Must.ErrorIs(expectedErr1.Get(t), err)
			t.Must.ErrorIs(expectedErr2.Get(t), err)
			t.Must.ErrorIs(expectedErr2.Get(t), err)
		})

		s.Then("errors.Is yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.Is(err, ErrType1{}))
			t.Must.False(errors.Is(err, ErrType2{}))
		})

		s.Then("errors.As yield false", func(t *testcase.T) {
			err := act(t)
			t.Must.False(errors.As(err, &ErrType1{}))
			t.Must.False(errors.As(err, &ErrType2{}))
		})

		s.And("the errors has a typed error value", func(s *testcase.Spec) {
			expectedErr2.LetValue(s, ErrType1{})

			s.Then("the named error value is returned", func(t *testcase.T) {
				t.Must.ErrorIs(expectedErr2.Get(t), act(t))
			})

			s.Then("errors.Is can find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.Is(err, ErrType1{}))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As can find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.As(err, &ErrType1{}))
				t.Must.False(errors.As(err, &ErrType2{}))
			})
		})

		s.And("the errors has multiple typed error value", func(s *testcase.Spec) {
			expectedErr2.LetValue(s, ErrType1{})
			expectedErr3.Let(s, func(t *testcase.T) error {
				return ErrType2{V: t.Random.Int()}
			})

			s.Then("returned error contains all typed error", func(t *testcase.T) {
				t.Must.ErrorIs(expectedErr2.Get(t), act(t))
				t.Must.ErrorIs(expectedErr3.Get(t), act(t))
			})

			s.Then("errors.Is can find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.Is(err, expectedErr2.Get(t)))
				t.Must.True(errors.Is(err, expectedErr3.Get(t)))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As can find the wrapped error", func(t *testcase.T) {
				err := act(t)
				t.Must.True(errors.As(err, &ErrType1{}))

				var gotErrWithAs ErrType2
				t.Must.True(errors.As(err, &gotErrWithAs))
				t.Must.NotNil(gotErrWithAs)
				t.Must.Equal(expectedErr3.Get(t), gotErrWithAs)
			})
		})

		s.And("but the error values are nil", func(s *testcase.Spec) {
			expectedErr1.LetValue(s, nil)
			expectedErr2.LetValue(s, nil)
			expectedErr3.LetValue(s, nil)

			s.Then("it will return with nil", func(t *testcase.T) {
				t.Must.Nil(act(t))
			})

			s.Then("errors.Is yield false", func(t *testcase.T) {
				err := act(t)
				t.Must.False(errors.Is(err, ErrType1{}))
				t.Must.False(errors.Is(err, ErrType2{}))
			})

			s.Then("errors.As yield false", func(t *testcase.T) {
				err := act(t)
				t.Must.False(errors.As(err, &ErrType1{}))
				t.Must.False(errors.As(err, &ErrType2{}))
			})
		})
	})
}
