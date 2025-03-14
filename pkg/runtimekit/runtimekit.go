package runtimekit

import (
	"fmt"
	"iter"
	"math"
	"reflect"
	"runtime"
	"strings"
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

// FuncOf returns the runtime function metadata for the given function.
//
// It takes any func type value as input and attempts to resolve it to a runtime.Func.
// If the input is not a function, it panics.
// If the input is nil, it returns nil.
func FuncOf(fn any) *runtime.Func {
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

// FuncInfo is the information of a function type.
//
// IT's naming is based on the go language spec's terminology.
// https://go.dev/ref/spec#Function_types
type FuncInfo struct {
	// ID is the function's identifier
	ID string
	// Receiver is the type's identifier in case of a method.
	Receiver string
	// Package is the name of the package where the function is declared.
	Package string
	// Import is the import path of the function's package.
	// Import includes the package name as well
	Import string
	// IsMethodValue indicates if the function is a method value.
	//
	// In Go, methods are functions associated with a specific type and require
	// a receiver (instance) to be called. When a method is assigned to a function
	// variable (e.g., `m.MyMethod`), Go creates a *method value* — a closure function
	// that "remembers" the receiver and can be invoked like a regular function.
	//
	// https://go.dev/ref/spec#Method_values
	IsMethodValue bool
}

func FuncInfoOf(fn *runtime.Func) FuncInfo {
	const ImportPathSeperator = "/"
	var info FuncInfo
	if fn == nil {
		return info
	}
	var (
		ids                  []string // [ package name ] | [ receiver type ] [ function name ]
		funcName             = fn.Name()
		indexOfLastSeperator = strings.LastIndex(funcName, ImportPathSeperator)
	)
	if 0 <= indexOfLastSeperator { // non top level package
		identifiers := funcName[indexOfLastSeperator+1:]
		ids = strings.Split(identifiers, ".")

		info.Import = funcName[:indexOfLastSeperator+1] + ids[0]
	} else { // top level stdlib
		ids = strings.Split(funcName, ".")
		info.Import = ids[0]
	}
	switch {
	case len(ids) == 2:
		info.Package = ids[0]
		info.ID = ids[1]
	case len(ids) == 3:
		info.Package = ids[0]
		info.ID = ids[2]
		receiver := ids[1]
		receiver = strings.TrimPrefix(receiver, "(")
		receiver = strings.TrimSuffix(receiver, ")")
		info.Receiver = receiver
	}

	const MethodValueSuffix = "-fm"
	if strings.HasSuffix(info.ID, MethodValueSuffix) {
		info.IsMethodValue = true
		info.ID = strings.TrimSuffix(info.ID, MethodValueSuffix)
	}

	return info
}
