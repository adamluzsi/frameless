package iterators_test

import (
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

var _ frameless.Iterator = iterators.NewSlice([]string{"A", "B", "C"})

func TestNewSlice_SliceGiven_SliceIterableAndValuesReturnedWithDecode(t *testing.T) {
	t.Parallel()

	i := iterators.NewSlice([]int{42, 4, 2})

	var v int

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&v))
	require.Equal(t, 42, v)

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&v))
	require.Equal(t, 4, v)

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&v))
	require.Equal(t, 2, v)

	require.False(t, i.Next())
	require.Nil(t, i.Err())
}

func TestNewSlice_Closed_ClosedErrorReturned(t *testing.T) {
	t.Parallel()

	i := iterators.NewSlice([]int{42, 4, 2})

	require.Nil(t, i.Close())

	require.False(t, i.Next())

	var v int
	require.Error(t, i.Decode(&v), "closed")
}

func TestNewSlice_ClosedCalledMultipleTimes_NoErrorReturned(t *testing.T) {
	t.Parallel()

	i := iterators.NewSlice([]int{42})

	for index := 0; index < 42; index++ {
		require.Nil(t, i.Close())
	}
}

func TestNewSlice_NotSliceGiven_PanicSent(t *testing.T) {
	t.Parallel()

	defer func() { require.Equal(t, "TypeError", recover()) }()

	iterators.NewSlice(42)
}

func TestNewSlice_SliceGivenButWrongTypeFetched_PanicSent(t *testing.T) {
	t.Parallel()

	i := iterators.NewSlice([]int{42})
	require.True(t, i.Next())

	var v string
	require.EqualError(t, i.Decode(&v), "reflect.Set: value of type int is not assignable to type string")
}
