package iterators_test

import (
	"errors"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

func TestWithCallback(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	s.When(`no callback is defined`, func(s *testcase.Spec) {
		s.Then(`it will execute iterator calls like it is not even there`, func(t *testcase.T) {
			expected := []int{1, 2, 3}
			input := iterators.NewSlice(expected)
			i := iterators.WithCallback(input, iterators.Callback{})

			var actually []int
			require.Nil(t, iterators.Collect(i, &actually))
			require.Equal(t, 3, len(actually))
			require.ElementsMatch(t, expected, actually)
		})
	})

	s.When(`OnClose callback is given`, func(s *testcase.Spec) {
		s.Then(`the callback receive the Close func call`, func(t *testcase.T) {
			var closeHook []string

			m := iterators.NewMock(iterators.NewSlice([]int{1, 2, 3}))
			m.StubClose = func() error {
				closeHook = append(closeHook, `during`)
				return nil
			}

			i := iterators.WithCallback(m, iterators.Callback{
				OnClose: func(closer io.Closer) error {
					closeHook = append(closeHook, `before`)
					err := closer.Close()
					closeHook = append(closeHook, `after`)
					return err
				},
			})

			require.Nil(t, i.Close())
			require.Equal(t, 3, len(closeHook))
			require.Equal(t, `before`, closeHook[0])
			require.Equal(t, `during`, closeHook[1])
			require.Equal(t, `after`, closeHook[2])
		})

		s.And(`error happen during closing in hook`, func(s *testcase.Spec) {
			s.And(`and the callback decide to forward the error`, func(s *testcase.Spec) {
				s.Then(`error received`, func(t *testcase.T) {
					expectedErr := errors.New(`boom`)

					m := iterators.NewMock(iterators.NewSlice([]int{1, 2, 3}))
					m.StubClose = func() error { return expectedErr }
					i := iterators.WithCallback(m, iterators.Callback{
						OnClose: func(closer io.Closer) error {
							return closer.Close()
						}})

					require.Equal(t, expectedErr, i.Close())
				})
			})

			s.And(`the callback decide to hide the error`, func(s *testcase.Spec) {
				s.Then(`error held back`, func(t *testcase.T) {
					expectedErr := errors.New(`boom`)

					m := iterators.NewMock(iterators.NewSlice([]int{1, 2, 3}))
					m.StubClose = func() error { return expectedErr }
					i := iterators.WithCallback(m, iterators.Callback{
						OnClose: func(closer io.Closer) error {
							_ = closer.Close()
							return nil
						}})

					require.Nil(t, i.Close())
				})
			})
		})

		s.And(`callback prevent call from being called`, func(s *testcase.Spec) {
			s.Then(`close will never happen`, func(t *testcase.T) {
				var closed bool
				m := iterators.NewMock(iterators.NewSlice([]int{1, 2, 3}))
				m.StubClose = func() error {
					closed = true
					return nil
				}

				i := iterators.WithCallback(m, iterators.Callback{
					OnClose: func(closer io.Closer) error {
						return nil // ignore closer explicitly
					},
				})

				require.Nil(t, i.Close())
				require.False(t, closed)
			})
		})
	})
}
