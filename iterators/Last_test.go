package iterators_test

import (
	"testing"

	"github.com/adamluzsi/frameless/errs"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func TestLast_NextValueDecodable_TheLastNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42
	var actually int

	i := iterators.NewMock(iterators.NewSlice([]int{4, 2, expected}))

	if err := iterators.Last(i, &actually); err != nil {
		t.Fatal(err)
	}

	require.Equal(t, expected, actually)
}

func TestLast_AfterLastValueDecoded_IteratorIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators.NewMock(iterators.NewSlice([]Entity{{Text: "hy!"}}))

	closed := false
	i.StubClose = func() error {
		closed = true
		return nil
	}

	if err := iterators.Last(i, &Entity{}); err != nil {
		t.Fatal(err)
	}

	require.True(t, closed)
}

func TestLast_WhenErrorOccursDuring(t *testing.T) {
	SharedErrCases(t, iterators.Last)
}

func TestLast_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	i := iterators.NewEmpty()

	require.Equal(t, errs.ErrNotFound, iterators.Last(i, &Entity{}))
}
