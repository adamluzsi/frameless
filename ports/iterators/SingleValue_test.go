package iterators_test

import (
	"fmt"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"math/rand"
	"testing"

	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

var _ iterators2.Iterator[any] = iterators2.SingleValue[any]("")

type ExampleStruct struct {
	Name string
}

var RandomName = fmt.Sprintf("%d", rand.Int())

func TestNewSingleElement_StructGiven_StructReceivedWithDecode(t *testing.T) {
	t.Parallel()

	var expected = ExampleStruct{Name: RandomName}

	i := iterators2.SingleValue[ExampleStruct](expected)
	defer i.Close()

	actually, found, err := iterators2.First[ExampleStruct](i)
	assert.Must(t).Nil(err)
	assert.Must(t).True(found)
	assert.Must(t).Equal(expected, actually)
}

func TestNewSingleElement_StructGivenAndNextCalledMultipleTimes_NextOnlyReturnTrueOnceAndStayFalseAfterThat(t *testing.T) {
	t.Parallel()

	var expected = ExampleStruct{Name: RandomName}

	i := iterators2.SingleValue(&expected)
	defer i.Close()

	assert.Must(t).True(i.Next())

	checkAmount := random.New(random.CryptoSeed{}).IntBetween(1, 100)
	for n := 0; n < checkAmount; n++ {
		assert.Must(t).False(i.Next())
	}

}

func TestNewSingleElement_CloseCalled_DecodeWarnsAboutThis(t *testing.T) {
	t.Parallel()

	i := iterators2.SingleValue(&ExampleStruct{Name: RandomName})
	i.Close()
	assert.Must(t).False(i.Next())
	assert.Must(t).Nil(i.Err())
}
