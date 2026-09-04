package memory_test

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/guard/guardcontract"
)

func ExampleLocker() {
	l := memory.NewLocker()

	ctx, err := l.Lock(context.Background())
	if err != nil {
		panic(err)
	}

	if err := l.Unlock(ctx); err != nil {
		panic(err)
	}
}

func TestLocker(t *testing.T) {
	guardcontract.Locker(memory.NewLocker()).Test(t)
}

func TestLockerFactory(t *testing.T) {
	guardcontract.LockerFactory[string, guard.Locker](memory.NewLockerFactory[string, guard.Locker]()).Test(t)
}

// TestLocker_waitingForATakenLockParksTheGoroutine pins that Lock waits by
// parking the goroutine instead of busy-spinning on TryLock.
//
// A spin loop keeps a CPU core at 100% for as long as the lock is held. In a
// test suite that starves every other goroutine, which is what made this
// package look like it hung rather than merely being slow.
func TestLocker_waitingForATakenLockParksTheGoroutine(t *testing.T) {
	l := memory.NewLocker()

	lockCtx, err := l.Lock(context.Background())
	assert.NoError(t, err)
	t.Cleanup(func() { _ = l.Unlock(lockCtx) })

	waitCtx, cancel := context.WithCancel(context.Background())

	var waiter sync.WaitGroup
	waiter.Add(1)
	// cancel first, wait second: the waiting Lock call only returns once its
	// context is done, so the reverse order would deadlock the cleanup.
	t.Cleanup(func() {
		cancel()
		waiter.Wait()
	})
	go func() {
		defer waiter.Done()
		if ctx, err := l.Lock(waitCtx); err == nil {
			_ = l.Unlock(ctx)
		}
	}()

	const waitingIn = "go.llib.dev/frameless/adapter/memory.(*Lock).Lock"

	assert.Eventually(t, 3*time.Second, func(it testing.TB) {
		states := goroutineStatesWithin(waitingIn)
		assert.NotEmpty(it, states, "expected a goroutine to be waiting inside Lock")

		for _, state := range states {
			assert.True(it, state != "running" && state != "runnable",
				"Lock is busy-spinning while it waits for the lock, instead of parking the goroutine",
				assert.MessageF("goroutine state: %s", state))
		}
	})
}

// goroutineStatesWithin reports the scheduler state of every goroutine whose
// current stack contains the given function.
func goroutineStatesWithin(fn string) []string {
	var buf []byte
	for size := 1 << 16; ; size *= 2 {
		buf = make([]byte, size)
		if n := runtime.Stack(buf, true); n < size {
			buf = buf[:n]
			break
		}
	}

	var states []string
	for _, block := range strings.Split(string(buf), "\n\n") {
		if !strings.Contains(block, fn) {
			continue
		}
		header, _, _ := strings.Cut(block, "\n")
		// the header reads like: "goroutine 36 [chan send, 2 minutes]:"
		_, state, ok := strings.Cut(header, "[")
		if !ok {
			continue
		}
		state, _, _ = strings.Cut(state, "]")
		state, _, _ = strings.Cut(state, ",")
		states = append(states, state)
	}
	return states
}

func TestNewLockerFactory_race(tt *testing.T) {
	t := testcase.NewT(tt)
	lf := memory.NewLockerFactory[string, guard.Locker]()

	const constKey = "const"
	testcase.Race(func() {
		lf.LockerFor(t.Random.String())
	}, func() {
		lf.LockerFor(t.Random.String())
	}, func() {
		lf.LockerFor(constKey)
	}, func() {
		lf.LockerFor(constKey)
	})
}
