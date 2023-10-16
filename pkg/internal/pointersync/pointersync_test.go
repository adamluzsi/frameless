package pointersync

import (
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"runtime"
	"testing"
)

func TestLocks_noGarbage(t *testing.T) {
	l := NewLocks()
	var fns []func()
	for i := 0; i < 3; i++ {
		var v int
		for i := 0; i < runtime.NumCPU(); i++ {
			fns = append(fns, func() {
				ptr := &v
				defer l.Sync(Key(ptr))()
				*ptr = 42
			})
		}
	}
	testcase.Race(func() {}, func() {}, fns...)
	assert.Equal(t, 0, len(l.Locks))
}
