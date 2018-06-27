package iterateover_test

import (
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/iterators/iterateover"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

func TestAndCollectAll_NonPointerValues(t *testing.T) {
	t.Parallel()

	var expected []int = []int{1, 2, 3, 4, 5}
	var actually []int

	i := iterators.NewForSlice(expected)

	require.Nil(t, iterateover.AndCollectAll(i, &actually))

	require.Equal(t, expected, actually)
}

func TestAndCollectAll_PointerValues(t *testing.T) {
	t.Parallel()

	var expected []*X = []*X{&X{"A"}, &X{"B"}, &X{"C"}, &X{"D"}}
	var actually []*X

	i := iterators.NewForSlice(expected)

	require.Nil(t, iterateover.AndCollectAll(i, &actually))

	require.Equal(t, expected, actually)
}

func TestAndCollectAll_IteratorResourceFailsForSomeReason_ErrReturned(t *testing.T) {
	t.Parallel()

	i := iterators.NewMock(iterators.NewForSlice([]int{42, 43, 44}))

	expectedDecodeError := errors.New("Boom Decode!")
	i.DecodeStub = func(interface{}) error { return expectedDecodeError }
	require.Error(t, expectedDecodeError, iterateover.AndCollectAll(i, &[]int{}))
	i.ResetDecode()

	expectedErrError := errors.New("Boom Err!")
	i.ErrStub = func() error { return expectedErrError }
	require.Error(t, expectedErrError, iterateover.AndCollectAll(i, &[]int{}))
}
