package iterators_test

import (
	"testing"

	"go.llib.dev/frameless/ports/iterators"

	"go.llib.dev/testcase/assert"
)

func TestLast_NextValueDecodable_TheLastNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42

	i := iterators.Stub[int](iterators.Slice[int]([]int{4, 2, expected}))

	actually, found, err := iterators.Last[int](i)
	assert.Must(t).Nil(err)
	assert.Must(t).True(found)
	assert.Must(t).Equal(expected, actually)
}

func TestLast_AfterLastValueDecoded_IteratorIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators.Stub[Entity](iterators.Slice[Entity]([]Entity{{Text: "hy!"}}))

	closed := false
	i.StubClose = func() error {
		closed = true
		return nil
	}

	_, _, err := iterators.Last[Entity](i)
	if err != nil {
		t.Fatal(err)
	}

	assert.Must(t).True(closed)
}

func TestLast_WhenErrorOccursDuring(t *testing.T) {
	FirstAndLastSharedErrorTestCases(t, iterators.Last[Entity])
}

func TestLast_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	_, found, err := iterators.Last[Entity](iterators.Empty[Entity]())
	assert.Must(t).Nil(err)
	assert.Must(t).False(found)
}
