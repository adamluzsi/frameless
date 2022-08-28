package iterators_test

import (
	"errors"
	"github.com/adamluzsi/frameless/pkg/iterators"
	"testing"

	"github.com/adamluzsi/testcase/assert"
)

func TestCount_andCountTotalIterations_IteratorGiven_AllTheRecordsCounted(t *testing.T) {
	t.Parallel()

	i := iterators.Slice[int]([]int{1, 2, 3})
	total, err := iterators.Count[int](i)
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(3, total)
}

func TestCount_errorOnCloseReturned(t *testing.T) {
	t.Parallel()

	s := iterators.Slice[int]([]int{1, 2, 3})
	m := iterators.Stub[int](s)

	expected := errors.New("boom")
	m.StubClose = func() error {
		return expected
	}

	_, err := iterators.Count[int](m)
	assert.Must(t).Equal(expected, err)
}
