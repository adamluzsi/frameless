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

func WaitWhile(condition func() bool) {
	DefaultWaiter.WaitWhile(condition)
}

// Waiter can also mean a someone/something who waits for a time, event, or opportunity.
// Waiter provides utility functionalities for waiting related use-cases.
type Waiter struct {
	SleepDuration time.Duration
	WaitTimeout   time.Duration
}

func (w Waiter) Wait() {
	times := runtime.NumCPU()
	for i := 0; i < times; i++ {
		runtime.Gosched()
		time.Sleep(w.SleepDuration)
	}
}

func (w Waiter) WaitWhile(condition func() bool) {
	initialTime := time.Now()
	finishTime := initialTime.Add(w.WaitTimeout)
	for time.Now().Before(finishTime) && condition() {
		w.Wait()
	}
}
