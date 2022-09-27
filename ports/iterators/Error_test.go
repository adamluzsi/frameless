package iterators_test

import (
	"errors"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"testing"

	"github.com/adamluzsi/testcase/assert"
)

var _ iterators2.Iterator[any] = iterators2.Error[any](errors.New("boom"))

func TestNewError_ErrorGiven_NotIterableIteratorReturnedWithError(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("Boom!")
	i := iterators2.Error[any](expectedError)
	assert.Must(t).False(i.Next())
	assert.Must(t).Nil(i.Value())
	assert.Must(t).NotNil(expectedError, i.Err())
	assert.Must(t).Nil(i.Close())
}
