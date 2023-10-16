package iterators_test

import (
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/ports/iterators"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestForEach(t *testing.T) {
	s := testcase.NewSpec(t)

	iter := testcase.Var[iterators.Iterator[int]]{ID: "frameless.Iterator"}
	fn := testcase.Var[func(int) error]{ID: "ForEach fn"}
	var subject = func(t *testcase.T) error {
		return iterators.ForEach[int](iter.Get(t), fn.Get(t))
	}

	s.When(`iterator has values`, func(s *testcase.Spec) {
		elements := testcase.Let(s, func(t *testcase.T) []int { return []int{1, 2, 3} })
		iter.Let(s, func(t *testcase.T) iterators.Iterator[int] { return iterators.Slice(elements.Get(t)) })

		s.And(`function block given`, func(s *testcase.Spec) {
			iteratedOnes := testcase.Let(s, func(t *testcase.T) map[int]struct{} { return make(map[int]struct{}) })
			fnErr := testcase.Let(s, func(t *testcase.T) error { return nil })

			fn.Let(s, func(t *testcase.T) func(int) error {
				return func(n int) error {
					iteratedOnes.Get(t)[n] = struct{}{}
					return fnErr.Get(t)
				}
			})

			s.Then(`it will iterate over all the elements without a problem`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				iterated := iteratedOnes.Get(t)
				for _, n := range elements.Get(t) {
					_, ok := iterated[n]
					assert.Must(t).True(ok, assert.Message(fmt.Sprintf(`expected that %d will be iterated by the function`, n)))
				}
			})

			s.And(`an error returned by the function`, func(s *testcase.Spec) {
				const expectedErr errorkit.Error = `boom`
				fnErr.Let(s, func(t *testcase.T) error { return expectedErr })

				s.Then(`it will return the error`, func(t *testcase.T) {
					t.Must.ErrorIs(expectedErr, subject(t))
				})

				s.Then(`it will cancel the iteration`, func(t *testcase.T) {
					_ = subject(t)
					t.Must.True(len(elements.Get(t)) > 1)
					t.Must.Equal(len(iteratedOnes.Get(t)), 1)
				})
			})

			var andAnErrorReturnedWhenIteratorBeingClosed = func(s *testcase.Spec) {
				s.And(`error returned when iterator being closed`, func(s *testcase.Spec) {
					const closeErr errorkit.Error = `boom on close`
					s.Before(func(t *testcase.T) {
						i := iterators.Stub(iter.Get(t))
						i.StubClose = func() error { return closeErr }
						iter.Set(t, i)
					})

					s.Then(`it will propagate back the error`, func(t *testcase.T) {
						t.Must.ErrorIs(closeErr, subject(t))
					})
				})
			}

			andAnErrorReturnedWhenIteratorBeingClosed(s)

			s.And(`break error returned from the block`, func(s *testcase.Spec) {
				fnErr.Let(s, func(t *testcase.T) error { return iterators.Break })

				s.Then(`it finish without an error`, func(t *testcase.T) {
					t.Must.Nil(subject(t))
				})

				s.Then(`it will cancel the iteration`, func(t *testcase.T) {
					_ = subject(t)
					t.Must.True(len(elements.Get(t)) > 1)
					t.Must.Equal(len(iteratedOnes.Get(t)), 1)
				})

				andAnErrorReturnedWhenIteratorBeingClosed(s)
			})
		})
	})
}

func TestForEach_CompatbilityWithEmptyInterface(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}

	var found []int
	assert.Must(t).Nil(iterators.ForEach[int](iterators.Slice[int](slice), func(n int) error {
		found = append(found, n)
		return nil
	}))

	assert.Must(t).ContainExactly(slice, found)
}
