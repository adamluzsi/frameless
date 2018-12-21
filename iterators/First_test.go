package iterators_test

import (
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func TestFirst_NextValueDecodable_TheFirstNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42
	var actually int

	i := iterators.NewMock(iterators.NewSlice([]int{expected, 4, 2}))

	if err := iterators.First(i, &actually); err != nil {
		t.Fatal(err)
	}

	require.Equal(t, expected, actually)
}

func TestFirst_AfterFirstValueDecoded_IteratorIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators.NewMock(iterators.NewSlice([]*Entity{&Entity{Text: "hy!"}}))

	closed := false
	i.StubClose = func() error {
		closed = true
		return nil
	}

	if err := iterators.First(i, &Entity{}); err != nil {
		t.Fatal(err)
	}

	require.True(t, closed)
}

func TestFirst_WhenErrorOccursDuring(t *testing.T) {
	SharedErrCases(t, iterators.First)
}

func TestFirst_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	i := iterators.NewEmpty()

	var ExpectedError error = iterators.ErrNoNextElement

	require.Equal(t, ExpectedError, iterators.First(i, &Entity{}))
}
