package iterators_test

import (
	"errors"
	"fmt"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"strings"
	"testing"

	"github.com/adamluzsi/testcase"
)

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	inputStream := testcase.Let(s, func(t *testcase.T) iterators2.Iterator[string] {
		return iterators2.Slice([]string{`a`, `b`, `c`})
	})
	transform := testcase.Var[iterators2.MapTransformFunc[string, string]]{ID: `iterators.MapTransformFunc`}

	subject := func(t *testcase.T) iterators2.Iterator[string] {
		return iterators2.Map(inputStream.Get(t), transform.Get(t))
	}

	s.When(`map used, the new iterator will have the changed values`, func(s *testcase.Spec) {
		transform.Let(s, func(t *testcase.T) iterators2.MapTransformFunc[string, string] {
			return func(in string) (string, error) {
				return strings.ToUpper(in), nil
			}
		})

		s.Then(`the new iterator will return values with enhanced by the map step`, func(t *testcase.T) {
			vs, err := iterators2.Collect[string](subject(t))
			t.Must.Nil(err)
			t.Must.ContainExactly([]string{`A`, `B`, `C`}, vs)
		})

		s.And(`some error happen during mapping`, func(s *testcase.Spec) {
			expectedErr := errors.New(`boom`)
			transform.Let(s, func(t *testcase.T) iterators2.MapTransformFunc[string, string] {
				return func(string) (string, error) {
					return "", expectedErr
				}
			})

			s.Then(`error returned`, func(t *testcase.T) {
				i := subject(t)
				t.Must.False(i.Next())
				t.Must.Equal(expectedErr, i.Err())
			})
		})

	})

	s.Describe(`map used in a daisy chain style`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) iterators2.Iterator[string] {
			toUpper := func(s string) (string, error) {
				return strings.ToUpper(s), nil
			}

			withIndex := func() func(s string) (string, error) {
				var index int
				return func(s string) (string, error) {
					defer func() { index++ }()
					return fmt.Sprintf(`%s%d`, s, index), nil
				}
			}

			i := inputStream.Get(t)
			i = iterators2.Map(i, toUpper)
			i = iterators2.Map(i, withIndex())

			return i
		}

		s.Then(`it will execute all the map steps in the final iterator composition`, func(t *testcase.T) {
			values, err := iterators2.Collect(subject(t))
			t.Must.Nil(err)
			t.Must.ContainExactly([]string{`A0`, `B1`, `C2`}, values)
		})
	})

	s.Describe(`proxy like behavior for underlying iterator object`, func(s *testcase.Spec) {
		inputStream.Let(s, func(t *testcase.T) iterators2.Iterator[string] {
			m := iterators2.Stub[string](iterators2.Empty[string]())
			m.StubErr = func() error {
				return errors.New(`ErrErr`)
			}
			m.StubClose = func() error {
				return errors.New(`ErrClose`)
			}
			return m
		})

		transform.Let(s, func(t *testcase.T) iterators2.MapTransformFunc[string, string] {
			return func(s string) (string, error) { return s, nil }
		})

		s.Then(`close is the underlying iterators's close return value`, func(t *testcase.T) {
			err := subject(t).Close()
			t.Must.NotNil(err)
			t.Must.Equal(`ErrClose`, err.Error())
		})

		s.Then(`Err is the underlying iterators's Err return value`, func(t *testcase.T) {
			err := subject(t).Err()
			t.Must.NotNil(err)
			t.Must.Equal(`ErrErr`, err.Error())
		})
	})
}