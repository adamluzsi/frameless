package iterators_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

func TestLast_NextValueDecodable_TheLastNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42
	var actually int

	i := iterators.NewMock(iterators.NewSlice([]int{4, 2, expected}))

	found, err := iterators.Last(i, &actually)
	require.Nil(t, err)
	require.True(t, found)
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

	_, err := iterators.Last(i, &Entity{})
	if err != nil {
		t.Fatal(err)
	}

	require.True(t, closed)
}

func TestLast_WhenErrorOccursDuring(t *testing.T) {
	FirstAndLastSharedErrorTestCases(t, iterators.Last)
}

func TestLast_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	found, err := iterators.Last(iterators.NewEmpty(), &Entity{})
	require.Nil(t, err)
	require.False(t, found)
}
