package iterateover_test

import (
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
