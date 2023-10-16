package iterators_test

import (
	"errors"
	"io"
	"testing"

	"go.llib.dev/frameless/ports/iterators"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestWithCallback(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	s.When(`no callback is defined`, func(s *testcase.Spec) {
		s.Then(`it will execute iterator calls like it is not even there`, func(t *testcase.T) {
			expected := []int{1, 2, 3}
			input := iterators.Slice(expected)
			i := iterators.WithCallback[int](input, iterators.Callback{})

			actually, err := iterators.Collect(i)
			assert.Must(t).Nil(err)
			assert.Must(t).Equal(3, len(actually))
			assert.Must(t).ContainExactly(expected, actually)
		})
	})

	s.When(`OnClose callback is given`, func(s *testcase.Spec) {
		s.Then(`the callback receive the Close func call`, func(t *testcase.T) {
			var closeHook []string

			m := iterators.Stub[int](iterators.Slice[int]([]int{1, 2, 3}))
			m.StubClose = func() error {
				closeHook = append(closeHook, `during`)
				return nil
			}

			i := iterators.WithCallback[int](m, iterators.Callback{
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

					m := iterators.Stub[int](iterators.Slice[int]([]int{1, 2, 3}))
					m.StubClose = func() error { return expectedErr }
					i := iterators.WithCallback[int](m, iterators.Callback{
						OnClose: func(closer io.Closer) error {
							return closer.Close()
						}})

					assert.Must(t).Equal(expectedErr, i.Close())
				})
			})

			s.And(`the callback decide to hide the error`, func(s *testcase.Spec) {
				s.Then(`error held back`, func(t *testcase.T) {
					expectedErr := errors.New(`boom`)

					m := iterators.Stub[int](iterators.Slice[int]([]int{1, 2, 3}))
					m.StubClose = func() error { return expectedErr }
					i := iterators.WithCallback[int](m, iterators.Callback{
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
				m := iterators.Stub[int](iterators.Slice[int]([]int{1, 2, 3}))
				m.StubClose = func() error {
					closed = true
					return nil
				}

				i := iterators.WithCallback[int](m, iterators.Callback{
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
