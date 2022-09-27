package iterators_test

import (
	"errors"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"io"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func TestWithCallback(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	s.When(`no callback is defined`, func(s *testcase.Spec) {
		s.Then(`it will execute iterator calls like it is not even there`, func(t *testcase.T) {
			expected := []int{1, 2, 3}
			input := iterators2.Slice(expected)
			i := iterators2.WithCallback[int](input, iterators2.Callback{})

			actually, err := iterators2.Collect(i)
			assert.Must(t).Nil(err)
			assert.Must(t).Equal(3, len(actually))
			assert.Must(t).ContainExactly(expected, actually)
		})
	})

	s.When(`OnClose callback is given`, func(s *testcase.Spec) {
		s.Then(`the callback receive the Close func call`, func(t *testcase.T) {
			var closeHook []string

			m := iterators2.Stub[int](iterators2.Slice[int]([]int{1, 2, 3}))
			m.StubClose = func() error {
				closeHook = append(closeHook, `during`)
				return nil
			}

			i := iterators2.WithCallback[int](m, iterators2.Callback{
				OnClose: func(closer io.Closer) error {
					closeHook = append(closeHook, `before`)
					err := closer.Close()
					closeHook = append(closeHook, `after`)
					return err
				},
			})

			assert.Must(t).Nil(i.Close())
			assert.Must(t).Equal(3, len(closeHook))
			assert.Must(t).Equal(`before`, closeHook[0])
			assert.Must(t).Equal(`during`, closeHook[1])
			assert.Must(t).Equal(`after`, closeHook[2])
		})

		s.And(`error happen during closing in hook`, func(s *testcase.Spec) {
			s.And(`and the callback decide to forward the error`, func(s *testcase.Spec) {
				s.Then(`error received`, func(t *testcase.T) {
					expectedErr := errors.New(`boom`)

					m := iterators2.Stub[int](iterators2.Slice[int]([]int{1, 2, 3}))
					m.StubClose = func() error { return expectedErr }
					i := iterators2.WithCallback[int](m, iterators2.Callback{
						OnClose: func(closer io.Closer) error {
							return closer.Close()
						}})

					assert.Must(t).Equal(expectedErr, i.Close())
				})
			})

			s.And(`the callback decide to hide the error`, func(s *testcase.Spec) {
				s.Then(`error held back`, func(t *testcase.T) {
					expectedErr := errors.New(`boom`)

					m := iterators2.Stub[int](iterators2.Slice[int]([]int{1, 2, 3}))
					m.StubClose = func() error { return expectedErr }
					i := iterators2.WithCallback[int](m, iterators2.Callback{
						OnClose: func(closer io.Closer) error {
							_ = closer.Close()
							return nil
						}})

					assert.Must(t).Nil(i.Close())
				})
			})
		})

		s.And(`callback prevent call from being called`, func(s *testcase.Spec) {
			s.Then(`close will never happen`, func(t *testcase.T) {
				var closed bool
				m := iterators2.Stub[int](iterators2.Slice[int]([]int{1, 2, 3}))
				m.StubClose = func() error {
					closed = true
					return nil
				}

				i := iterators2.WithCallback[int](m, iterators2.Callback{
					OnClose: func(closer io.Closer) error {
						return nil // ignore closer explicitly
					},
				})

				assert.Must(t).Nil(i.Close())
				assert.Must(t).False(closed)
			})
		})
	})
}
