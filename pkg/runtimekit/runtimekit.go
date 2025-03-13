package runtimekit

import (
	"fmt"
	"iter"
	"math"
	"reflect"
	"runtime"
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

func OverStack() iter.Seq[runtime.Frame] {
	return func(yield func(runtime.Frame) bool) {
		var (
			programCounters []uintptr
			size            int
			nCallers        int
		)

		// Dynamically expand the buffer until it can hold all callers.
		for i := 0; ; i++ {
			size = int(math.Pow(2, float64(i)))
			programCounters = make([]uintptr, size)
			nCallers = runtime.Callers(1 /* skip this frame */, programCounters)

			if nCallers < len(programCounters) {
				break // The buffer is sufficiently large
			}
		}

		framesIter := runtime.CallersFrames(programCounters[:nCallers])

	tracing:
		for {
			frame, more := framesIter.Next()
			if !more {
				break tracing
			}
			if isException(frame) {
				continue
			}
			if !yield(frame) {
				break tracing
			}
		}
	}
}

func Stack() []runtime.Frame {
	var stack []runtime.Frame
	for frame := range OverStack() {
		stack = append(stack, frame)
	}
	return stack
}

func Func(fn any) *runtime.Func {
	if fn == nil {
		return nil
	}
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		panic(fmt.Sprintf("non-function kind: %s", v.Kind()))
	}
	pc := uintptr(v.UnsafePointer())
	return runtime.FuncForPC(pc)
}
