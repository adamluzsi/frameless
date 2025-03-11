package runtimekit

import (
	"runtime"
	"strings"
	"sync/atomic"
	"unsafe"
)

// ArchBitSize returns the current Architecture's supported bit size.
//
// In this context, `unsafe.Sizeof(ptr)`
// returns the size (in bytes) of a variable or zero-sized type.
//
// For pointer types like `uintptr`,
// this will return either 4 (for a 32-bit architecture) or 8 (for a 64-bit architecture),
// because those are the standard sizes that pointers occupy in memory for these architectures.
//
// By multiplying the result by 8 (`sizeOfPtr*8`),
// we're converting bytes to bits, because there are 8 bits in one byte.
//
// In other words:
//
// - If your machine is 32 bit (and thus `uintptr` size is 4 bytes) - `4 * 8 = 32`
// - If your machine is 64 bit (and thus `uintptr` size is 8 bytes) - `8 * 8 = 64`
//
// This multiplication gives you the number of bits that a pointer uses, which can be used as an indication of whether the system is running a 32-bit or 64-bit version of the Go runtime.
func ArchBitSize() int {
	var ptr uintptr
	sizeOfPointer := unsafe.Sizeof(ptr)
	return int(sizeOfPointer * 8)
}

var exceptionsIndex int64
var exceptions = map[int64]func(runtime.Frame) bool{}

func RegisterTraceException(isException func(runtime.Frame) bool) func() {
	var index int64
	for range 1024 {
		index = atomic.AddInt64(&exceptionsIndex, 1)
		if _, ok := exceptions[index]; !ok {
			break
		}
	}
	if _, ok := exceptions[index]; ok {
		panic("errorkit trace expection list is probably full")
	}
	exceptions[index] = isException
	return func() { delete(exceptions, index) }
}

var _ = RegisterTraceException(func(f runtime.Frame) bool {
	return strings.Contains(f.Function, "runtimekit.") ||
		strings.Contains(f.Function, "runtime.") ||
		strings.Contains(f.Function, "testing.")
})

func Stack() []runtime.Frame {
	programCounters := make([]uintptr, 1024)
	n_callers := runtime.Callers(1, programCounters)
	frames := runtime.CallersFrames(programCounters[:n_callers])
	var vs []runtime.Frame

tracing:
	for more := true; more; {
		var frame runtime.Frame
		frame, more = frames.Next()

		for _, exception := range exceptions {
			if exception(frame) {
				continue tracing
			}
		}

		vs = append(vs, frame)
	}

	return vs
}
