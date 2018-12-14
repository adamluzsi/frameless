package iterators_test

import (
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

func TestFilter(t *testing.T) {
	t.Run("Filter", func(t *testing.T) {
		t.Parallel()

		t.Run("given the iterator has set of elements", func(t *testing.T) {
			originalInput := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
			iterator := func() frameless.Iterator { return iterators.NewSlice(originalInput) }

			t.Run("when filter allow everything", func(t *testing.T) {
				i := iterators.Filter(iterator(), func(e frameless.Entity) bool { return true })
				require.NotNil(t, i)

				var numbers []int
				require.Nil(t, iterators.CollectAll(i, &numbers))
				require.Equal(t, originalInput, numbers)
			})

			t.Run("when filter disallow part of the value stream", func(t *testing.T) {
				i := iterators.Filter(iterator(), func(e frameless.Entity) bool { return 5 < e.(int) })
				require.NotNil(t, i)

				var numbers []int
				require.Nil(t, iterators.CollectAll(i, &numbers))
				require.Equal(t, []int{6, 7, 8, 9}, numbers)
			})

			t.Run("but iterator encounter an exception", func(t *testing.T) {
				srcI := iterator

				t.Run("during Decode", func(t *testing.T) {

					iterator = func() frameless.Iterator {
						m := iterators.NewMock(srcI())
						m.StubDecode = func(frameless.Entity) error { return fmt.Errorf("Boom!") }
						return m
					}

					t.Run("it is expect to report the error with the Err method", func(t *testing.T) {
						i := iterators.Filter(iterator(), func(e frameless.Entity) bool { return true })
						require.NotNil(t, i)
						require.False(t, i.Next())
						require.Equal(t, i.Err(), fmt.Errorf("Boom!"))
					})
				})

				t.Run("during somewhere which stated in the src iterator Err", func(t *testing.T) {

					iterator = func() frameless.Iterator {
						m := iterators.NewMock(srcI())
						m.StubErr = func() error { return fmt.Errorf("Boom!!") }
						return m
					}

					t.Run("it is expect to report the error with the Err method", func(t *testing.T) {
						i := iterators.Filter(iterator(), func(e frameless.Entity) bool { return true })
						require.NotNil(t, i)
						require.Equal(t, i.Err(), fmt.Errorf("Boom!!"))
					})
				})

				t.Run("during Closing the iterator", func(t *testing.T) {

					iterator = func() frameless.Iterator {
						m := iterators.NewMock(srcI())
						m.StubClose = func() error { return fmt.Errorf("Boom!!!") }
						return m
					}

					t.Run("it is expect to report the error with the Err method", func(t *testing.T) {
						i := iterators.Filter(iterator(), func(e frameless.Entity) bool { return true })
						require.NotNil(t, i)
						require.Nil(t, i.Err())
						require.Equal(t, i.Close(), fmt.Errorf("Boom!!!"))
					})
				})

			})

		})

	})
}
