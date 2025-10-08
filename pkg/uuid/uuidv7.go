package uuid

import (
	"fmt"
	"io"
	"sync"
	"time"

	"go.llib.dev/testcase/clock"
)

// MakeV7 will create a UUID v7.
func MakeV7() (UUID, error) {
	return v7.Make()
}

var v7 = V7{}

// V7 represents a UUID version 7, designed for monotonic ordering and time-based sorting.
// Timestamp precision is millisecond, with 12-bit nanosecond extension for intra-millisecond uniqueness.
//
// https://www.rfc-editor.org/rfc/rfc9562#section-4.3
//
//	-------------------------------------------
//	field       bits value
//	-------------------------------------------
//	unix_ts_ms  48   timestamp (ms since epoch)
//	ver          4   0x7
//	rand_a      12   random or monotonic counter
//	var          2   0b10
//	rand_b      62   cryptographically secure random
//	-------------------------------------------
//	total       128
//	-------------------------------------------
type V7 struct {
	Random io.Reader
	RandA  func(now time.Time) (UInt12, error)
}

type UInt12 = uint16

const maxUInt12 = 1<<12 - 1 // 4095

func truncateToInt12(n uint16) uint16 {
	return n & 0xFFF // 4096 possible values
}

var defaultV7RandA = func() func(now time.Time) (UInt12, error) {
	if isTimeNanosecPrecise() {
		return V7SubMillisecondRandA
	}
	// if current time implementation is not nanosec precise
	// then we fall back to a per process specific counter approach.
	return V7MonotonicCounter()
}()

func (g *V7) counter() func(time.Time) (UInt12, error) {
	if g.RandA != nil {
		return g.RandA
	}
	return defaultV7RandA
}

func (g *V7) Make() (UUID, error) {
	var uuid UUID

	now := clock.Now()
	// Use 48-bit millisecond timestamp (required by spec).
	g.setTimestamp(&uuid, now)

	if err := g.setRandA(&uuid, now); err != nil {
		return uuid, err
	}

	if err := fillWithRandom(uuid[8:], g.Random); err != nil {
		return uuid, err
	}

	uuid.setVersion(7) // set version to v7
	uuid.setVariant(2) // Set variant (RFC 4122)

	return uuid, nil
}

func (g *V7) setRandA(uuid *UUID, now time.Time) error {
	counter, err := g.counter()(now)
	if err != nil {
		return fmt.Errorf("error in uuid.V7#Counter: %w", err)
	}
	counter = truncateToInt12(counter) // 4096 possible values
	uuid[6] = byte(counter >> 8)       // high 4 bits
	uuid[7] = byte(counter)
	return nil
}

func (g *V7) setTimestamp(uuid *UUID, now time.Time) {
	millis := now.UnixMilli()
	uuid[0] = byte(millis >> 40)
	uuid[1] = byte(millis >> 32)
	uuid[2] = byte(millis >> 24)
	uuid[3] = byte(millis >> 16)
	uuid[4] = byte(millis >> 8)
	uuid[5] = byte(millis)
}

// V7MonotonicCounter returns a function that generates a monotonically increasing 16-bit counter
// within the current process. Using the provided timestamp, it ensures that counters are reset across millisecs.
//
// This counter is designed to help ensure ordering of UUID v7 values within a single process,
// especially when nanosecond precision is available. It resets at the start of each millisecond
// and increments for subsequent calls within the same millisecond.
//
// Note: This counter is not shared across processes or machines—it's strictly local to the calling process.
// For globally consistent ordering across distributed systems, consider using V7NanosecCounter instead.
//
// The returned function is safe for concurrent use within the same process.
func V7MonotonicCounter() func(now time.Time) (UInt12, error) {
	var (
		mutex      sync.Mutex
		counter    uint16
		prevMillis int64
	)
	return func(now time.Time) (UInt12, error) {
		mutex.Lock()
		defer mutex.Unlock()
		millis := now.UnixMilli()
		if millis <= prevMillis {
			if counter == maxUInt12 {
				return counter, nil
			}
			counter++
		} else {
			prevMillis = millis
			counter = 0
		}
		return counter, nil
	}
}

// V7SubMillisecondRandA is a time package based nanosec rand_a counter for sub-millisec precision.
//
// When the system supports nanosecond precision,
// using nanoseconds as a counter in a distributed environment becomes a natural and reliable choice.
//
// Since time is generally synchronised across servers,
// nanoseconds can act as a globally consistent sequence;
// ensuring clear, ordered issuing of UUID v7 values even in a cloud native setup.
//
// The implementation is based on RFC 9562 Method 3: "Replace Leftmost Random Bits with Increased Clock Precision"
//
// This method uses sub-millisecond timestamp precision to fill the rand_a field,
// providing time-ordered values with sub-millisecond precision as specified in RFC 9562 Section 6.2.
//
// This approach ensures monotonic ordering within the same millisecond while
// utilizing the full precision available from the system clock.
// The resulting UUIDs maintain temporal ordering even when generated
// in rapid succession within the same millisecond.
func V7SubMillisecondRandA(now time.Time) (UInt12, error) {
	// now truncated down to the start of its millisecond
	startOfMs := now.Truncate(time.Millisecond)
	// difference gives a Duration representing nanoseconds elapsed since start of ms
	elapsed := now.Sub(startOfMs)
	// nanoseconds since start of millisecond
	ns := elapsed.Nanoseconds()
	// `ns` can range beetween 0 to 999.999,
	// since 1 millisec is 1000000 nanosec.
	unit := float64(ns) / float64(time.Millisecond)
	randA := UInt12(unit * maxUInt12)
	randA = truncateToInt12(randA)
	return randA, nil
}

func isTimeNanosecPrecise() bool {
	var cs = map[UInt12]struct{}{}
	for i := 0; i < 128; i++ {
		c, _ := V7SubMillisecondRandA(time.Now())
		cs[c] = struct{}{}
	}
	return 1 < len(cs)
}
