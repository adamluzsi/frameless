package zerokit

import (
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"runtime"
	"testing"
	"time"
)

func TestInit_noGarbage(t *testing.T) {
	var fns []func()
	for i := 0; i < 3; i++ {
		var v int
		for i := 0; i < runtime.NumCPU(); i++ {
			fns = append(fns, func() {
				Init(&v, func() int {
					return 42
				})
			})
		}
	}

	testcase.Race(func() {}, func() {}, fns...)

	assert.EventuallyWithin(5*time.Second).Assert(t, func(it assert.It) {
		var count int
		initlcks.Mutex.RLock()
		count = len(initlcks.Locks)
		initlcks.Mutex.RUnlock()
		it.Must.Equal(0, count)
	})
}
