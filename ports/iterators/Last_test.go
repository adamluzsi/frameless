package iterators_test

import (
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"testing"

	"github.com/adamluzsi/testcase/assert"
)

func TestLast_NextValueDecodable_TheLastNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42

	i := iterators2.Stub[int](iterators2.Slice[int]([]int{4, 2, expected}))

	actually, found, err := iterators2.Last[int](i)
	assert.Must(t).Nil(err)
	assert.Must(t).True(found)
	assert.Must(t).Equal(expected, actually)
}

func TestLast_AfterLastValueDecoded_IteratorIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators2.Stub[Entity](iterators2.Slice[Entity]([]Entity{{Text: "hy!"}}))

	closed := false
	i.StubClose = func() error {
		closed = true
		return nil
	}

	_, _, err := iterators2.Last[Entity](i)
	if err != nil {
		t.Fatal(err)
	}

	assert.Must(t).True(closed)
}

func TestLast_WhenErrorOccursDuring(t *testing.T) {
	FirstAndLastSharedErrorTestCases(t, iterators2.Last[Entity])
}

func TestLast_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	_, found, err := iterators2.Last[Entity](iterators2.Empty[Entity]())
	assert.Must(t).Nil(err)
	assert.Must(t).False(found)
}
