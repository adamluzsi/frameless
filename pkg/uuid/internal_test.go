package uuid

import (
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/testcase/assert"
)

func Test_setVersion(t *testing.T) {
	id, err := MakeV4()
	assert.NoError(t, err)

	for i := range iterkit.IntRange(1, 15) {
		id.setVersion(i)

		assert.Equal(t, i, id.Version())
	}
}

func Test_setVariant(t *testing.T) {
	id, err := MakeV4()
	assert.NoError(t, err)

	for i := range iterkit.IntRange(0, 3) {
		id.setVariant(i)

		assert.Equal(t, i, id.Variant(), "unexpected variant after setting the value")
	}
}
