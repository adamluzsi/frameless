package iterators_test

import (
	"context"
	"testing"
	"time"

	"go.llib.dev/frameless/ports/iterators"
	"go.llib.dev/testcase/assert"
)

func ExampleHead() {
	infStream := iterators.Func[int](func() (v int, ok bool, err error) {
		return 42, true, nil
	})

	i := iterators.Head(infStream, 3)

	vs, err := iterators.Collect(i)
	_, _ = vs, err // []{42, 42, 42}, nil
}

func TestHead(t *testing.T) {
	t.Run("less", func(t *testing.T) {
		i := iterators.Slice([]int{1, 2, 3})
		i = iterators.Head(i, 2)
		vs, err := iterators.Collect(i)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2}, vs)
	})
	t.Run("more", func(t *testing.T) {
		i := iterators.Slice([]int{1, 2, 3})
		i = iterators.Head(i, 5)
		vs, err := iterators.Collect(i)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, vs)
	})
	t.Run("closes", func(t *testing.T) {
		var (
			expErr  = rnd.Error()
			closedN int
		)

		stub := iterators.Stub(iterators.Slice([]int{1, 2, 3, 4, 5}))
		stub.StubClose = func() error {
			closedN++
			return expErr
		}

		i := iterators.Head[int](stub, 3)

		vs, err := iterators.Collect(i)
		assert.ErrorIs(t, expErr, err)
		assert.Equal(t, []int{1, 2, 3}, vs)
		assert.ErrorIs(t, expErr, i.Close())
		assert.Equal(t, 1, closedN,
			"expected that close only called once")
	})
	t.Run("err", func(t *testing.T) {
		expErr := rnd.Error()
		i := iterators.Error[int](expErr)
		i = iterators.Head(i, 42)
		assert.False(t, i.Next())
		assert.ErrorIs(t, expErr, i.Err())
		assert.NoError(t, i.Close())
	})
	t.Run("inf iterator", func(t *testing.T) {
		assert.Within(t, time.Second, func(ctx context.Context) {
			infStream := iterators.Func[int](func() (v int, ok bool, err error) {
				if ctx.Err() != nil {
					return v, false, nil
				}
				return 42, true, nil
			})

			i := iterators.Head(infStream, 3)

			vs, err := iterators.Collect(i)
			assert.NoError(t, err)
			assert.Equal(t, []int{42, 42, 42}, vs)
		})
	})
}
