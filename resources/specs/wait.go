package specs

import (
	"runtime"
	"sync/atomic"
	"time"
)

var DefaultWaiter = Waiter{
	SleepDuration: time.Microsecond,
	WaitTimeout:   time.Minute,
}

func Wait() {
	DefaultWaiter.Wait()
}

func WaitForLen(length func() int, expectedMinimumLen int) {
	DefaultWaiter.WaitForLen(length, expectedMinimumLen)
}

// Waiter can also mean a someone/something who waits for a time, event, or opportunity.
type Waiter struct {
	SleepDuration time.Duration
	WaitTimeout   time.Duration
}

func (w Waiter) WaitForLen(length func() int, expectedMinimumLen int) {
	timer := time.NewTimer(w.WaitTimeout)
	defer timer.Stop()
	var timeIsUp int32
	go func() {
		<-timer.C
		atomic.AddInt32(&timeIsUp, 1)
	}()
	for timeIsUp == 0 {
		if expectedMinimumLen <= length() {
			return
		}
		w.Wait()
	}
}

func (w Waiter) Wait() {
	times := runtime.NumCPU()
	for i := 0; i < times; i++ {
		runtime.Gosched()
		time.Sleep(w.SleepDuration)
	}
}
