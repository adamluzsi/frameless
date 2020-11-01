package iterators_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

func TestForEach(t *testing.T) {
	s := testcase.NewSpec(t)

	var subject = func(t *testcase.T) error {
		return iterators.ForEach(t.I(`iterator`).(iterators.Interface), t.I(`fn`))
	}

	s.When(`iterator has values`, func(s *testcase.Spec) {
		s.Let(`elements`, func(t *testcase.T) interface{} { return []int{1, 2, 3} })
		s.Let(`iterator`, func(t *testcase.T) interface{} { return iterators.NewSlice(t.I(`elements`)) })

		s.And(`function block given`, func(s *testcase.Spec) {
			s.Let(`iterated ones`, func(t *testcase.T) interface{} { return make(map[int]struct{}) })
			s.Let(`fn.Err`, func(t *testcase.T) interface{} { return nil })
			s.Let(`fn`, func(t *testcase.T) interface{} {
				return func(n int) error {
					t.I(`iterated ones`).(map[int]struct{})[n] = struct{}{}
					err, _ := t.I(`fn.Err`).(error)
					return err
				}
			})

			s.Then(`it will iterate over all the elements without a problem`, func(t *testcase.T) {
				require.Nil(t, subject(t))

				iterated := t.I(`iterated ones`).(map[int]struct{})
				for _, n := range t.I(`elements`).([]int) {
					_, ok := iterated[n]
					require.True(t, ok, fmt.Sprintf(`expected that %d will be iterated by the function`, n))
				}
			})

			s.And(`an error returned by the function`, func(s *testcase.Spec) {
				const errMsg = `boom`
				s.Let(`fn.Err`, func(t *testcase.T) interface{} { return errors.New(errMsg) })

				s.Then(`it will return the error`, func(t *testcase.T) {
					require.EqualError(t, subject(t), errMsg)
				})

				s.Then(`it will cancel the iteration`, func(t *testcase.T) {
					_ = subject(t)
					require.True(t, len(t.I(`elements`).([]int)) > 1)
					require.Len(t, t.I(`iterated ones`).(map[int]struct{}), 1)
				})
			})

			var andErrorReturnedWhenNextElementIsDecoded = func(s *testcase.Spec) {
				s.And(`error returned when next element is decoded`, func(s *testcase.Spec) {
					const decodeErrMsg = `boom on decode`
					s.Before(func(t *testcase.T) {
						i := iterators.NewMock(t.I(`iterator`).(iterators.Interface))
						i.StubDecode = func(interface{}) error { return errors.New(decodeErrMsg) }
						t.Let(`iterator`, i)
					})

					s.Then(`it will return the decode error back`, func(t *testcase.T) {
						require.EqualError(t, subject(t), decodeErrMsg)
					})
				})
			}

			andErrorReturnedWhenNextElementIsDecoded(s)

			var andAnErrorReturnedWhenIteratorBeingClosed = func(s *testcase.Spec) {
				s.And(`error returned when iterator being closed`, func(s *testcase.Spec) {
					const closeErrMsg = `boom on close`
					s.Before(func(t *testcase.T) {
						i := iterators.NewMock(t.I(`iterator`).(iterators.Interface))
						i.StubClose = func() error { return errors.New(closeErrMsg) }
						t.Let(`iterator`, i)
					})

					s.Then(`it will propagate back the error`, func(t *testcase.T) {
						require.EqualError(t, subject(t), closeErrMsg)
					})

					andErrorReturnedWhenNextElementIsDecoded(s)
				})
			}

			andAnErrorReturnedWhenIteratorBeingClosed(s)

			s.And(`break error returned from the block`, func(s *testcase.Spec) {
				s.Let(`fn.Err`, func(t *testcase.T) interface{} { return iterators.Break })

				s.Then(`it finish without an error`, func(t *testcase.T) {
					require.Nil(t, subject(t))
				})

				s.Then(`it will cancel the iteration`, func(t *testcase.T) {
					_ = subject(t)
					require.True(t, len(t.I(`elements`).([]int)) > 1)
					require.Len(t, t.I(`iterated ones`).(map[int]struct{}), 1)
				})

				andAnErrorReturnedWhenIteratorBeingClosed(s)
			})
		})
	})
}

func TestForEach_CompatbilityWithEmptyInterface(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}

	var found []int
	require.Nil(t, iterators.ForEach(iterators.NewSlice(slice), func(i interface{}) error {
		n, ok := i.(int)
		require.True(t, ok, `expected that under the empty interface it will be an int`)
		found = append(found, n)
		return nil
	}))

	require.ElementsMatch(t, slice, found)
}
