package frameless_test

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

func TestDecoderFunc_Decode(t *testing.T) {
	expected := random.New(random.CryptoSeed{}).String()

	decoder := frameless.DecoderFunc[string](func(ptr *string) error {
		return reflects.Link(expected, ptr)
	})

	var actual string
	assert.Must(t).Nil(decoder.Decode(&actual))
	assert.Must(t).Equal(expected, actual)
}
