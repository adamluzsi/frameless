package zerokit

import (
	"testing"

	"go.llib.dev/testcase/assert"
)

func Test_pointerKey_unique(t *testing.T) {
	var keys = map[uintptr]struct{}{}

	var v int
	for i := 0; i < 1000; i++ {
		ptr := &v
		keys[pointerKey(ptr)] = struct{}{}

		for j := 0; j < 1000; j++ {
			keys[pointerKey(&v)] = struct{}{}
		}
	}

	assert.Equal(t, 1, len(keys), "expected that all the pointer creation only made a single key")
}
