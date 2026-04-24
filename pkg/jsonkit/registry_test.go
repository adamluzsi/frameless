package jsonkit_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/jsonkit"
	"go.llib.dev/testcase/assert"
)

func TestLookupTypeID_smoke(t *testing.T) {
	type X struct{}

	id, ok := jsonkit.LookupTypeID[X]()
	assert.False(t, ok)
	assert.Empty(t, id)

	const exp = "test::x"
	td := jsonkit.RegisterTypeID[X](exp)

	id, ok = jsonkit.LookupTypeID[X]()
	assert.True(t, ok)
	assert.Equal(t, exp, id)

	td()

	id, ok = jsonkit.LookupTypeID[X]()
	assert.False(t, ok)
	assert.Empty(t, id)
}
