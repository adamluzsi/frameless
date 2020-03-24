package iterators_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
)

var _ iterators.Interface = iterators.NewSingleElement("")

type ExampleStruct struct {
	Name string
}

var RandomName = fixtures.RandomString(42)

func TestNewSingleElement_StructGiven_StructReceivedWithDecode(t *testing.T) {
	t.Parallel()

	var expected = ExampleStruct{Name: RandomName}
	var actually ExampleStruct

	i := iterators.NewSingleElement(expected)
	defer i.Close()

	found, err := iterators.First(i, &actually)
	require.Nil(t, err)
	require.True(t, found)
	require.Equal(t, expected, actually)
}

func TestNewSingleElement_StructGivenAndNextCalledMultipleTimes_NextOnlyReturnTrueOnceAndStayFalseAfterThat(t *testing.T) {
	t.Parallel()

	var expected = ExampleStruct{Name: RandomName}

	i := iterators.NewSingleElement(&expected)
	defer i.Close()

	require.True(t, i.Next())

	checkAmount := fixtures.RandomIntByRange(1, 100)
	for n := 0; n < checkAmount; n++ {
		require.False(t, i.Next())
	}

}

func TestNewSingleElement_NextCalled_DecodeShouldDoNothing(t *testing.T) {
	t.Parallel()

	var expected = ExampleStruct{Name: RandomName}
	var actually ExampleStruct

	i := iterators.NewSingleElement(&expected)
	defer i.Close()
	i.Next()
	i.Next()

	require.Nil(t, i.Decode(&actually))
	require.NotEqual(t, expected, actually)

}

func TestNewSingleElement_CloseCalled_DecodeWarnsAboutThis(t *testing.T) {
	t.Parallel()

	i := iterators.NewSingleElement(&ExampleStruct{Name: RandomName})
	i.Close()

	require.Error(t, i.Decode(&ExampleStruct{}), "closed")

}
