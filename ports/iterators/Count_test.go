package iterators_test

import (
	"errors"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"testing"

	"github.com/adamluzsi/testcase/assert"
)

func TestCount_andCountTotalIterations_IteratorGiven_AllTheRecordsCounted(t *testing.T) {
	t.Parallel()

	i := iterators2.Slice[int]([]int{1, 2, 3})
	total, err := iterators2.Count[int](i)
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(3, total)
}

func TestCount_errorOnCloseReturned(t *testing.T) {
	t.Parallel()

	s := iterators2.Slice[int]([]int{1, 2, 3})
	m := iterators2.Stub[int](s)

	expected := errors.New("boom")
	m.StubClose = func() error {
		return expected
	}

	_, err := iterators2.Count[int](m)
	assert.Must(t).Equal(expected, err)
}
