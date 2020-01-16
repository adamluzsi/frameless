package iterators_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	subject := func(t *testcase.T) iterators.Iterator {
		return iterators.Map(t.I(`input stream`).(iterators.Iterator),
			t.I(`transform`).(iterators.MapTransformFunc))
	}

	s.Let(`input stream`, func(t *testcase.T) interface{} {
		return iterators.NewSlice([]string{`a`, `b`, `c`})
	})

	s.When(`map used, the new iterator will have the changed values`, func(s *testcase.Spec) {
		s.Let(`transform`, func(t *testcase.T) interface{} {
			return func(d iterators.Decoder, ptr interface{}) error {
				var s string
				if err := d.Decode(&s); err != nil {
					return err
				}
				*ptr.(*string) = strings.ToUpper(s)
				return nil
			}
		})

		s.Then(`the new iterator will return values with enhanced by the map step`, func(t *testcase.T) {
			var values []string
			require.Nil(t, iterators.Collect(subject(t), &values))
			require.ElementsMatch(t, []string{`A`, `B`, `C`}, values)
		})

		s.And(`some error happen during mapping`, func(s *testcase.Spec) {
			s.Let(`transform`, func(t *testcase.T) interface{} {
				return func(d iterators.Decoder, ptr interface{}) error {
					return errors.New(`boom`)
				}
			})

			s.Then(`error returned`, func(t *testcase.T) {
				i := subject(t)
				require.True(t, i.Next())

				var s string
				err := i.Decode(&s)
				require.Error(t, err)
				require.Equal(t, `boom`, err.Error())
			})
		})

	})

	s.Describe(`map used in a daisy chain style`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) iterators.Iterator {

			toUpper := func(d iterators.Decoder, ptr interface{}) error {
				var s string
				require.Nil(t, d.Decode(&s))
				s = strings.ToUpper(s)
				*ptr.(*string) = s
				return nil
			}

			withIndex := func() func(d iterators.Decoder, ptr interface{}) error {
				var index int

				return func(d iterators.Decoder, ptr interface{}) error {
					defer func() { index++ }()
					var s string
					require.Nil(t, d.Decode(&s))
					*ptr.(*string) = fmt.Sprintf(`%s%d`, s, index)
					return nil
				}
			}

			i := t.I(`input stream`).(iterators.Iterator)
			i = iterators.Map(i, toUpper)
			i = iterators.Map(i, withIndex())

			return i
		}

		s.Then(`it will execute all the map steps in the final iterator composition`, func(t *testcase.T) {
			var values []string
			require.Nil(t, iterators.Collect(subject(t), &values))
			require.ElementsMatch(t, []string{`A0`, `B1`, `C2`}, values)
		})
	})

	s.Describe(`proxy like behavior for underlying iterator object`, func(s *testcase.Spec) {
		s.Let(`input stream`, func(t *testcase.T) interface{} {
			m := iterators.NewMock(iterators.NewEmpty())
			m.StubErr = func() error {
				return errors.New(`ErrErr`)
			}
			m.StubClose = func() error {
				return errors.New(`ErrClose`)
			}
			return m
		})

		s.Let(`transform`, func(t *testcase.T) interface{} {
			return func(d iterators.Decoder, ptr interface{}) error { return d.Decode(ptr) }
		})

		s.Then(`close is the underlying iterators's close return value`, func(t *testcase.T) {
			err := subject(t).Close()
			require.Error(t, err)
			require.Equal(t, `ErrClose`, err.Error())
		})

		s.Then(`Err is the underlying iterators's Err return value`, func(t *testcase.T) {
			err := subject(t).Err()
			require.Error(t, err)
			require.Equal(t, `ErrErr`, err.Error())
		})
	})

}
