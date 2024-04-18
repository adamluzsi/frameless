package iterators_test

import (
	"errors"
	"go.llib.dev/testcase/random"
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
			i := iterators.WithCallback[int](input)

			actually, err := iterators.Collect(i)
			assert.Must(t).Nil(err)
			assert.Must(t).Equal(3, len(actually))
			assert.Must(t).ContainExactly(expected, actually)
		})

		s.Then(`if actually no option is given, it returns the original iterator`, func(t *testcase.T) {
			expected := []int{1, 2, 3}
			input := iterators.Slice(expected)
			i := iterators.WithCallback[int](input)
			assert.Equal(t, input, i)
			actually, err := iterators.Collect(i)
			assert.Must(t).Nil(err)
			assert.Must(t).Equal(3, len(actually))
			assert.Must(t).ContainExactly(expected, actually)
		})
	})

	s.When(`OnClose callback is given`, func(s *testcase.Spec) {
		s.Then(`the callback is called after the iterator.eClose`, func(t *testcase.T) {
			var closeHook []string

			m := iterators.Stub[int](iterators.Slice[int]([]int{1, 2, 3}))
			m.StubClose = func() error {
				closeHook = append(closeHook, `during`)
				return nil
			}

			callbackErr := random.New(random.CryptoSeed{}).Error()

			i := iterators.WithCallback[int](m,
				iterators.OnClose(func() error {
					closeHook = append(closeHook, `after`)
					return callbackErr
				}),
			)

			assert.Must(t).ErrorIs(callbackErr, i.Close())
			assert.Must(t).Equal(2, len(closeHook))
			assert.Must(t).Equal(`during`, closeHook[0])
			assert.Must(t).Equal(`after`, closeHook[1])
		})

		s.And(`error happen during closing in hook`, func(s *testcase.Spec) {
			s.And(`and the callback has no issue`, func(s *testcase.Spec) {
				s.Then(`error received`, func(t *testcase.T) {
					expectedErr := errors.New(`boom`)

					m := iterators.Stub[int](iterators.Slice[int]([]int{1, 2, 3}))
					m.StubClose = func() error { return expectedErr }
					i := iterators.WithCallback[int](m,
						iterators.OnClose(func() error {
							return nil
						}))

					assert.Must(t).Equal(expectedErr, i.Close())
				})
			})
		})
	})
}

func TestCallbackOnClose(t *testing.T) {
	var closed bool
	expErr := random.New(random.CryptoSeed{}).Error()
	iter := iterators.Slice([]int{1, 2, 3})
	iter = iterators.WithCallback(iter, iterators.OnClose(func() error {
		closed = true
		return expErr
	}))

	vs, err := iterators.Collect(iter)
	assert.ErrorIs(t, err, expErr)
	assert.Equal(t, []int{1, 2, 3}, vs)
	assert.True(t, closed)
}
