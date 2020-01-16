package iterators_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

func ExampleFilter() error {
	var iter iterators.Interface
	iter = iterators.NewSlice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	iter = iterators.Filter(iter, func(n int) bool { return n > 2 })

	defer iter.Close()
	for iter.Next() {
		var n int

		if err := iter.Decode(&n); err != nil {
			return err
		}

		fmt.Println(n)
	}

	return iter.Err()
}

func TestFilter(t *testing.T) {
	t.Run("Filter", func(t *testing.T) {
		t.Parallel()

		t.Run("given the iterator has set of elements", func(t *testing.T) {
			originalInput := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
			iterator := func() iterators.Interface { return iterators.NewSlice(originalInput) }

			t.Run("when filter allow everything", func(t *testing.T) {
				i := iterators.Filter(iterator(), func(interface{}) bool { return true })
				require.NotNil(t, i)

				var numbers []int
				require.Nil(t, iterators.Collect(i, &numbers))
				require.Equal(t, originalInput, numbers)
			})

			t.Run("when filter disallow part of the value stream", func(t *testing.T) {
				i := iterators.Filter(iterator(), func(n int) bool { return 5 < n })
				require.NotNil(t, i)

				var numbers []int
				require.Nil(t, iterators.Collect(i, &numbers))
				require.Equal(t, []int{6, 7, 8, 9}, numbers)
			})

			t.Run(`when filter function specify a struct type`, func(t *testing.T) {
				type T struct{ Int int }
				s := []T{{Int: 1}, {Int: 2}, {Int: 3}, {Int: 4}}
				i := iterators.Filter(iterators.NewSlice(s), func(t T) bool { return 2 < t.Int })

				var res []T
				require.Nil(t, iterators.Collect(i, &res))
				require.NotNil(t, res)
				require.Equal(t, 2, len(res))
				require.ElementsMatch(t, res, []T{{Int: 3}, {Int: 4}})
			})

			t.Run(`when filter function specify a pointer type`, func(t *testing.T) {
				t.Skip()
				type T struct{ Int int }
				s := []*T{{Int: 1}, {Int: 2}, {Int: 3}, {Int: 4}}
				i := iterators.Filter(iterators.NewSlice(s), func(t *T) bool { return 2 < t.Int })

				var res []*T
				require.Nil(t, iterators.Collect(i, &res))
				require.NotNil(t, res)
				require.Equal(t, 2, len(res))
				require.ElementsMatch(t, res, []*T{{Int: 3}, {Int: 4}})
			})

			t.Run("but iterator encounter an exception", func(t *testing.T) {
				srcI := iterator

				t.Run("during Decode", func(t *testing.T) {

					iterator = func() iterators.Interface {
						m := iterators.NewMock(srcI())
						m.StubDecode = func(interface{}) error { return fmt.Errorf("Boom!") }
						return m
					}

					t.Run("it is expect to report the error with the Err method", func(t *testing.T) {
						i := iterators.Filter(iterator(), func(interface{}) bool { return true })
						require.NotNil(t, i)
						require.False(t, i.Next())
						require.Equal(t, i.Err(), fmt.Errorf("Boom!"))
					})
				})

				t.Run("during somewhere which stated in the iterator iterator Err", func(t *testing.T) {

					iterator = func() iterators.Interface {
						m := iterators.NewMock(srcI())
						m.StubErr = func() error { return fmt.Errorf("Boom!!") }
						return m
					}

					t.Run("it is expect to report the error with the Err method", func(t *testing.T) {
						i := iterators.Filter(iterator(), func(interface{}) bool { return true })
						require.NotNil(t, i)
						require.Equal(t, i.Err(), fmt.Errorf("Boom!!"))
					})
				})

				t.Run("during Closing the iterator", func(t *testing.T) {

					iterator = func() iterators.Interface {
						m := iterators.NewMock(srcI())
						m.StubClose = func() error { return fmt.Errorf("Boom!!!") }
						return m
					}

					t.Run("it is expect to report the error with the Err method", func(t *testing.T) {
						i := iterators.Filter(iterator(), func(interface{}) bool { return true })
						require.NotNil(t, i)
						require.Nil(t, i.Err())
						require.Equal(t, i.Close(), fmt.Errorf("Boom!!!"))
					})
				})

			})

		})

	})
}

func BenchmarkFilter(b *testing.B) {
	for i := 0; i < b.N; i++ {

		b.StopTimer()
		var inputs []int
		for i := 0; i < 1024; i++ {
			inputs = append(inputs, rand.Intn(1000))
		}
		srcIter := iterators.NewSlice(inputs)
		b.StartTimer()

		_, err := iterators.Count(iterators.Filter(srcIter, func(n int) bool { return n > 500 }))
		require.Nil(b, err)

	}
}

func BenchmarkFilter_implementedWithPipe(b *testing.B) {
	for i := 0; i < b.N; i++ {

		b.StopTimer()
		var inputs []int
		for i := 0; i < 1024; i++ {
			inputs = append(inputs, rand.Intn(1000))
		}
		srcIter := iterators.NewSlice(inputs)

		r, w := iterators.NewPipe()

		go func() {
			defer srcIter.Close()
			defer w.Close()
			for srcIter.Next() {
				var n int
				_ = srcIter.Decode(&n)

				if n > 500 {
					_ = w.Encode(&n)
				}
			}
			w.Error(srcIter.Err())
		}()

		b.StartTimer()

		for r.Next() {
			var n int
			require.Nil(b, r.Decode(&n))
		}
		require.Nil(b, r.Err())
		_ = r.Close()

	}

}
