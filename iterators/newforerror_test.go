package iterators_test

import (
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func TestNewForError_ErrorGiven_NotIterableIteratorReturnedWithError(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("Boom!")
	i := iterators.NewForError(expectedError)
	require.False(t, i.Next())
	require.Nil(t, i.Decode(nil))
	require.Error(t, expectedError, i.Err())
	require.Nil(t, i.Close())
}
