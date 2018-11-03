package iterators_test

import (
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func TestAndCollectAll_NonPointerValues(t *testing.T) {
	t.Parallel()

	var expected []int = []int{1, 2, 3, 4, 5}
	var actually []int

	i := iterators.NewSlice(expected)

	require.Nil(t, iterators.CollectAll(i, &actually))

	require.Equal(t, expected, actually)
}

func TestAndCollectAll_PointerValues(t *testing.T) {
	t.Parallel()

	var expected []*Entity = []*Entity{&Entity{"A"}, &Entity{"B"}, &Entity{"C"}, &Entity{"D"}}
	var actually []*Entity

	i := iterators.NewSlice(expected)

	require.Nil(t, iterators.CollectAll(i, &actually))

	require.Equal(t, expected, actually)
}

func TestAndCollectAll_IteratorResourceFailsForSomeReason_ErrReturned(t *testing.T) {
	t.Parallel()

	i := iterators.NewMock(iterators.NewSlice([]int{42, 43, 44}))

	expectedDecodeError := errors.New("Boom Decode!")
	i.StubDecode = func(interface{}) error { return expectedDecodeError }
	require.Error(t, expectedDecodeError, iterators.CollectAll(i, &[]int{}))
	i.ResetDecode()

	expectedErrError := errors.New("Boom Err!")
	i.StubErr = func() error { return expectedErrError }
	require.Error(t, expectedErrError, iterators.CollectAll(i, &[]int{}))
}
