package iterators_test

import (
	"testing"

	"github.com/adamluzsi/frameless/ports/iterators"

	"github.com/adamluzsi/testcase/assert"
)

var _ iterators.Iterator[string] = iterators.Slice([]string{"A", "B", "C"})

func TestNewSlice_SliceGiven_SliceIterableAndValuesReturnedWithDecode(t *testing.T) {
	t.Parallel()

	i := iterators.Slice([]int{42, 4, 2})

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal(42, i.Value())

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal(4, i.Value())

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal(2, i.Value())

	assert.Must(t).False(i.Next())
	assert.Must(t).Nil(i.Err())
}

func TestNewSlice_ClosedCalledMultipleTimes_NoErrorReturned(t *testing.T) {
	t.Parallel()

	i := iterators.Slice([]int{42})

	for index := 0; index < 42; index++ {
		assert.Must(t).Nil(i.Close())
	}
}
