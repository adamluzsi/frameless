package repro_test

import (
	"bytes"
	"context"
	"fmt"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/clock"
	"go.llib.dev/testcase/clock/timecop"
)

// 1. T#Eventually copies *t (copyT := *t), which reads T.goroutines without its lock,
// while a finished t.Go job still writes it in Group.jobDone, even after Job#Wait returned.
func TestRepro_EventuallyRacesWithFinishedGoJob(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Test("", func(t *testcase.T) {
		job := t.Go(func(ctx context.Context) error { return nil })
		job.Wait()
		t.Eventually(func(t *testcase.T) {})
	})
}

// 2. T#Eventually copies *t, which reads T.teardown by value without its lock,
// while another goroutine registers a cleanup (e.g. a lazy let var with t.Cleanup).
func TestRepro_EventuallyRacesWithCleanup(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Test("", func(t *testcase.T) {
		go t.Cleanup(func() {})
		t.Eventually(func(t *testcase.T) {})
	})
}

// 3. Random#Read (thus UUID, Bytes, ...) uses the shared rand.Source without Random's mutex.
func TestRepro_RandomReadIsNotLocked(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Test("", func(t *testcase.T) {
		testcase.Race(func() { _ = t.Random.UUID() }, func() { _ = t.Random.UUID() })
	})
}

// 4. clock.After sends on an unbuffered channel, so when the receiver chose another select case,
// the goroutine (and its time travel subscription) blocks forever.
func TestRepro_ClockAfterLeaksWhenNotReceived(t *testing.T) {
	label := pprof.Labels("repro", "clock.After")
	pprof.Do(context.Background(), label, func(context.Context) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		select {
		case <-ctx.Done():
		case <-clock.After(time.Millisecond):
		}
	})
	time.Sleep(100 * time.Millisecond) // the timer has fired by now
	if n := numGoroutine(`"repro":"clock.After"`); n != 0 {
		t.Fatalf("expected the clock.After goroutine to end, but %d is still running", n)
	}
}

// 5. TestTicker's ticking and handleTimeTravel call each other recursively,
// so every time travel deepens the ticker goroutine's stack and never unwinds it.
func TestRepro_TickerStackGrowsWithTimeTravel(t *testing.T) {
	var ticker *clock.Ticker
	pprof.Do(context.Background(), pprof.Labels("repro", "ticker"), func(context.Context) {
		ticker = clock.NewTicker(time.Hour)
	})
	defer ticker.Stop()
	depth := func() int {
		var buf bytes.Buffer
		_ = pprof.Lookup("goroutine").WriteTo(&buf, 2)
		for _, g := range strings.Split(buf.String(), "\n\n") {
			if strings.Contains(g, "TestTicker).ticking") {
				return strings.Count(g, "TestTicker).handleTimeTravel")
			}
		}
		return -1
	}
	time.Sleep(10 * time.Millisecond)
	before := depth()
	for range 20 {
		timecop.Travel(t, time.Second)
		time.Sleep(time.Millisecond)
	}
	after := depth()
	t.Logf("handleTimeTravel frames before=%d after 20 time travels=%d", before, after)
	assert.Equal(t, before, after, "expected the ticker's stack depth to stay the same")
}

func numGoroutine(label string) int {
	var buf bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&buf, 1)
	var total, count int
	for _, line := range strings.Split(buf.String(), "\n") {
		if n, err := fmt.Sscanf(line, "%d @", &count); err == nil && n == 1 {
			continue
		}
		if strings.HasPrefix(line, "# labels: ") && strings.Contains(line, label) {
			total += count
		}
	}
	return total
}
