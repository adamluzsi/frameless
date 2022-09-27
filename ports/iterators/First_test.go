package iterators_test

import (
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"testing"

	"github.com/adamluzsi/testcase/assert"
)

func TestFirst_NextValueDecodable_TheFirstNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42
	i := iterators2.Slice([]int{expected, 4, 2})

	actually, found, err := iterators2.First[int](i)
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(expected, actually)
	assert.Must(t).True(found)
}

func TestFirst_AfterFirstValue_IteratorIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators2.Stub[Entity](iterators2.Slice[Entity]([]Entity{{Text: "hy!"}}))

	closed := false
	i.StubClose = func() error {
		closed = true
		return nil
	}

	_, _, err := iterators2.First[Entity](i)
	if err != nil {
		t.Fatal(err)
	}
	assert.Must(t).True(closed)
}

func TestFirst_errors(t *testing.T) {
	FirstAndLastSharedErrorTestCases(t, iterators2.First[Entity])
}

func TestFirst_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	_, found, err := iterators2.First[Entity](iterators2.Empty[Entity]())
	assert.Must(t).Nil(err)
	assert.Must(t).False(found)
}
