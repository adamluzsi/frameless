package iterators_test

import (
	"testing"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

type ExampleStruct struct {
	Name string
}

func TestNewForSingleElement_StructGiven_StructReceivedWithDecode(t *testing.T) {
	t.Parallel()

	var expected ExampleStruct = ExampleStruct{Name: randomdata.SillyName()}
	var actually ExampleStruct

	i := iterators.NewForSingleElement(&expected)
	defer i.Close()

	iterators.DecodeNext(i, &actually)

	require.Equal(t, expected, actually)
}

func TestNewForSingleElement_StructGivenAndNextCalledMultipleTimes_NextOnlyReturnTrueOnceAndStayFalseAfterThat(t *testing.T) {
	t.Parallel()

	var expected ExampleStruct = ExampleStruct{Name: randomdata.SillyName()}

	i := iterators.NewForSingleElement(&expected)
	defer i.Close()

	require.True(t, i.Next())

	checkAmount := randomdata.Number(1, 100)
	for n := 0; n < checkAmount; n++ {
		require.False(t, i.Next())
	}

}

func TestNewForSingleElement_NextCalled_DecodeShouldDoNothing(t *testing.T) {
	t.Parallel()

	var expected ExampleStruct = ExampleStruct{Name: randomdata.SillyName()}
	var actually ExampleStruct

	i := iterators.NewForSingleElement(&expected)
	defer i.Close()
	i.Next()
	i.Next()

	require.Nil(t, i.Decode(&actually))
	require.NotEqual(t, expected, actually)

}

func TestNewForSingleElement_CloseCalled_DecodeWarnsAboutThis(t *testing.T) {
	t.Parallel()

	i := iterators.NewForSingleElement(&ExampleStruct{Name: randomdata.SillyName()})
	i.Close()

	require.Error(t, i.Decode(&ExampleStruct{}), "closed")

}
