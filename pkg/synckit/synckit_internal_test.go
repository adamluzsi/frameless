package synckit

import (
	"runtime"
	"strconv"
	"testing"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestLocks_noGarbage(t *testing.T) {
	var l RWLockerFactory[string]
	var key string = "foo-bar-baz"

	var fns []func()
	for i := 0; i < runtime.NumCPU(); i++ {
		ik := strconv.Itoa(i)

		fns = append(fns, func() {
			m := l.RWLocker(key)
			m.Lock()
			defer m.Unlock()
		})
		fns = append(fns, func() {
			m := l.RWLocker(key)
			m.RLock()
			defer m.RUnlock()
		})
		fns = append(fns, func() {
			m := l.RWLocker(ik)
			m.Lock()
			defer m.Unlock()
		})
	}
	testcase.Race(func() {}, func() {}, fns...)

	assert.Equal(t, 0, len(l.locks))
}
