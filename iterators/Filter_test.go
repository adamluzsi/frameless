package iterators_test

import (
	"fmt"
	"log"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
)

func ExampleFilter() {
	var iter iterators.Interface
	iter = iterators.NewSlice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	iter = iterators.Filter(iter, func(n int) bool { return n > 2 })

	defer iter.Close()
	for iter.Next() {
		var n int

		if err := iter.Decode(&n); err != nil {
			log.Fatal(err)
		}

		_ = n
	}

	if err := iter.Err(); err != nil {
		log.Fatal(err)
	}
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
	s := testcase.NewSpec(b)

	var logic = func(n int) bool {
		return n > 500
	}

	s.Let(`iter`, func(t *testcase.T) interface{} {
		var values []int
		for i := 0; i < 1024; i++ {
			values = append(values, fixtures.Random.IntN(1000))
		}
		return iterators.NewSlice(values)
	})

	s.Let(`pipe-iter`, func(t *testcase.T) interface{} {
		srcIter := t.I(`iter`).(iterators.Interface)
		r, w := iterators.NewPipe()

		go func() {
			defer srcIter.Close()
			defer w.Close()

			for srcIter.Next() {
				var n int
				_ = srcIter.Decode(&n)

				if logic(n) {
					_ = w.Encode(n)
				}
			}
			w.Error(srcIter.Err())
		}()
		return r
	})

	s.Let(`filter-iter`, func(t *testcase.T) interface{} {
		return iterators.Filter(t.I(`iter`).(iterators.Interface), logic)
	})

	s.Before(func(t *testcase.T) {
		t.I(`filter-iter`) // eager load
		t.I(`pipe-iter`)   // eager load
	})

	s.Test(`filter iterator with pipe iterator`, func(t *testcase.T) {
		iter := t.I(`pipe-iter`).(iterators.Interface)
		for iter.Next() {
			var n int
			require.Nil(b, iter.Decode(&n))
		}

		require.Nil(b, iter.Err())
		require.Nil(b, iter.Close())
	})

	s.Test(`filter iterator with filter iterator`, func(t *testcase.T) {
		iter := t.I(`filter-iter`).(iterators.Interface)
		for iter.Next() {
			var n int
			require.Nil(b, iter.Decode(&n))
		}

		require.Nil(b, iter.Err())
		require.Nil(b, iter.Close())
	})
}

func TestFilter_decoderUsesConcreteType(t *testing.T) {
	stub := &StubIterator{
		Decoder: frameless.DecoderFunc(func(ptr interface{}) error {
			i := ptr.(*int)
			*i = 42
			return nil
		}),
	}

	iter := iterators.Filter(stub, func(i int) bool { return true })

	var i int
	iter.Next()
	require.Nil(t, iter.Decode(&i))
	require.Equal(t, 42, i)
}
