package frameless_test

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

func TestDecoderFunc_Decode(t *testing.T) {
	expected := fixtures.Random.String()

	decoder := frameless.DecoderFunc(func(ptr interface{}) error {
		return reflects.Link(expected, ptr)
	})

	var actual string
	require.Nil(t, decoder.Decode(&actual))
	require.Equal(t, expected, actual)
}
