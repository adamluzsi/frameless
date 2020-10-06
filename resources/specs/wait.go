package specs

import (
	"runtime"
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
	initialTime := time.Now()
	finishTime := initialTime.Add(w.WaitTimeout)
	expectationMet := func() bool { return expectedMinimumLen <= length() }
	for time.Now().Before(finishTime) && !expectationMet() {
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
