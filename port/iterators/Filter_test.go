package iterators_test

import (
	"fmt"
	"log"
	"testing"

	"go.llib.dev/frameless/port/iterators"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func ExampleFilter() {
	var iter iterators.Iterator[int]
	iter = iterators.Slice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	iter = iterators.Filter[int](iter, func(n int) bool { return n > 2 })

	defer iter.Close()
	for iter.Next() {
		n := iter.Value()
		_ = n
	}
	if err := iter.Err(); err != nil {
		log.Fatal(err)
	}
}

func TestFilter(t *testing.T) {
	t.Run("Filter", func(t *testing.T) {

		t.Run("given the iterator has set of elements", func(t *testing.T) {
			originalInput := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
			iterator := func() iterators.Iterator[int] { return iterators.Slice[int](originalInput) }

			t.Run("when filter allow everything", func(t *testing.T) {
				i := iterators.Filter(iterator(), func(int) bool { return true })
				assert.Must(t).NotNil(i)

				numbers, err := iterators.Collect[int](i)
				assert.Must(t).Nil(err)
				assert.Must(t).Equal(originalInput, numbers)
			})

			t.Run("when filter disallow part of the value stream", func(t *testing.T) {
				i := iterators.Filter(iterator(), func(n int) bool { return 5 < n })
				assert.Must(t).NotNil(i)

				numbers, err := iterators.Collect[int](i)
				assert.Must(t).Nil(err)
				assert.Must(t).Equal([]int{6, 7, 8, 9}, numbers)
			})

			t.Run("but iterator encounter an exception", func(t *testing.T) {
				srcI := iterator

				t.Run("during somewhere which stated in the iterator iterator Err", func(t *testing.T) {

					iterator = func() iterators.Iterator[int] {
						m := iterators.Stub(srcI())
						m.StubErr = func() error { return fmt.Errorf("Boom!!") }
						return m
					}

					t.Run("it is expect to report the error with the Err method", func(t *testing.T) {
						i := iterators.Filter[int](iterator(), func(int) bool { return true })
						assert.Must(t).NotNil(i)
						assert.Must(t).Equal(i.Err(), fmt.Errorf("Boom!!"))
					})
				})

				t.Run("during Closing the iterator", func(t *testing.T) {

					iterator = func() iterators.Iterator[int] {
						m := iterators.Stub(srcI())
						m.StubClose = func() error { return fmt.Errorf("Boom!!!") }
						return m
					}

					t.Run("it is expect to report the error with the Err method", func(t *testing.T) {
						i := iterators.Filter(iterator(), func(int) bool { return true })
						assert.Must(t).NotNil(i)
						assert.Must(t).Nil(i.Err())
						assert.Must(t).Equal(i.Close(), fmt.Errorf("Boom!!!"))
					})
				})
			})
		})
	})
}

func BenchmarkFilter(b *testing.B) {
	var logic = func(n int) bool {
		return n > 500
	}

	rnd := random.New(random.CryptoSeed{})

	var values []int
	for i := 0; i < 1024; i++ {
		values = append(values, rnd.IntN(1000))
	}

	makeIter := func() iterators.Iterator[int] {
		return iterators.Filter[int](iterators.Slice[int](values), logic)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		func() {
			iter := makeIter()
			defer iter.Close()
			for iter.Next() {
				//
			}
		}()
	}
}
