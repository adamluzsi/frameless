package uuid

import (
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/uuidkit"
	"go.llib.dev/testcase/assert"
)

func Test_setVersion(t *testing.T) {
	id, err := MakeV4()
	assert.NoError(t, err)

	for i := range iterkit.IntRange(1, 15) {
		setVersion(&id, i)

		assert.Equal(t, i, uuidkit.Version(id))
	}
}

func Test_setVariant(t *testing.T) {
	id, err := MakeV4()
	assert.NoError(t, err)

	for i := range iterkit.IntRange(0, 3) {
		setVariant(&id, i)

		assert.Equal(t, i, uuidkit.Variant(id), "unexpected variant after setting the value")
	}
}
