package iterators_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

func TestFirst_NextValueDecodable_TheFirstNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42
	var actually int

	i := iterators.NewMock(iterators.NewSlice([]int{expected, 4, 2}))

	found, err := iterators.First(i, &actually)
	require.Nil(t, err)
	require.Equal(t, expected, actually)
	require.True(t, found)
}

func TestFirst_AfterFirstValueDecoded_IteratorIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators.NewMock(iterators.NewSlice([]Entity{{Text: "hy!"}}))

	closed := false
	i.StubClose = func() error {
		closed = true
		return nil
	}

	_, err := iterators.First(i, &Entity{})
	if err != nil {
		t.Fatal(err)
	}
	require.True(t, closed)
}

func TestFirst_errors(t *testing.T) {
	FirstAndLastSharedErrorTestCases(t, iterators.First)
}

func TestFirst_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	found, err := iterators.First(iterators.NewEmpty(), &Entity{})
	require.Nil(t, err)
	require.False(t, found)
}
