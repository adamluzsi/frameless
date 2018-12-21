package iterators_test

import (
	"github.com/adamluzsi/frameless"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func TestDecodeNext_IteratorGiven_ValidDecoderReturnedThanCanDecodeTheFirstValueFromTheIterator(t *testing.T) {
	t.Parallel()

	var expected int = 42
	var actually int

	i := iterators.NewSlice([]int{expected, 4, 2})
	defer i.Close()

	if err := iterators.DecodeNext(i, &actually); err != nil {
		t.Fatal(err)
	}

	require.Equal(t, expected, actually)
}

func TestDecodeNext_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	i := iterators.NewEmpty()

	require.Equal(t, frameless.ErrNotFound, iterators.DecodeNext(i, &Entity{}))
}
