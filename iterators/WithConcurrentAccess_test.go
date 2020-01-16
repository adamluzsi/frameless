package iterators_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func TestWithConcurrentAccess(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test(`it will protect against concurrent access`, func(t *testcase.T) {
		var i iterators.Iterator
		i = iterators.NewSlice([]int{1, 2})
		i = iterators.WithConcurrentAccess(i)
		require.True(t, i.Next())

		var wg sync.WaitGroup
		wg.Add(1)
		defer wg.Wait()
		go func() {
			defer wg.Done()

			require.True(t, i.Next())
			var v int
			require.Nil(t, i.Decode(&v))
			require.Equal(t, 2, v)
		}()

		var v int
		require.Nil(t, i.Decode(&v))
		require.Equal(t, 1, v)
	})

	s.Test(`classic behavior`, func(t *testcase.T) {
		var i iterators.Iterator
		i = iterators.NewSlice([]int{1, 2})
		i = iterators.WithConcurrentAccess(i)

		var vs []int
		require.Nil(t, iterators.Collect(i, &vs))
		require.ElementsMatch(t, []int{1, 2}, vs)
	})

	s.Test(`proxy like behavior for underlying iterator object`, func(t *testcase.T) {
		m := iterators.NewMock(iterators.NewEmpty())
		m.StubErr = func() error {
			return errors.New(`ErrErr`)
		}
		m.StubClose = func() error {
			return errors.New(`ErrClose`)
		}
		i := iterators.WithConcurrentAccess(m)

		err := i.Close()
		require.Error(t, err)
		require.Equal(t, `ErrClose`, err.Error())

		err = i.Err()
		require.Error(t, err)
		require.Equal(t, `ErrErr`, err.Error())
	})
}
