package iterators_test

import (
	"testing"

	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/testcase/assert"
)

func TestFirst_NextValueDecodable_TheFirstNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42
	i := iterators.Slice([]int{expected, 4, 2})

	actually, found, err := iterators.First[int](i)
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(expected, actually)
	assert.Must(t).True(found)
}

func TestFirst_AfterFirstValue_IteratorIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators.Stub[Entity](iterators.Slice[Entity]([]Entity{{Text: "hy!"}}))

	closed := false
	i.StubClose = func() error {
		closed = true
		return nil
	}

	_, _, err := iterators.First[Entity](i)
	if err != nil {
		t.Fatal(err)
	}
	assert.Must(t).True(closed)
}

func TestFirst_errors(t *testing.T) {
	FirstAndLastSharedErrorTestCases(t, iterators.First[Entity])
}

func TestFirst_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	_, found, err := iterators.First[Entity](iterators.Empty[Entity]())
	assert.Must(t).Nil(err)
	assert.Must(t).False(found)
}
