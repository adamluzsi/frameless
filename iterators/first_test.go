package iterators_test

import (
	"errors"
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

func TestFirst_WhenErrorOccursDuringClosing_ErrorReturned(t *testing.T) {
	t.Parallel()

	expected := errors.New("Boom!")

	i := iterators.NewMock(iterators.NewSlice([]*Entity{&Entity{Text: "hy!"}}))
	i.StubClose = func() error { return expected }

	require.Equal(t, expected, iterators.First(i, &Entity{}))
}

func TestFirst_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	i := iterators.NewEmpty()

	require.Equal(t, iterators.ErrNoNextElement, iterators.First(i, &Entity{}))
}
