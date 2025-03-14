package runtimekit

import (
	"runtime"
	"strings"
	"sync/atomic"
)

var exceptionsIndex int64
var exceptions = map[int64]func(runtime.Frame) bool{}

// RegisterFrameException ensures that runtimekit globally ignores frames that match the given exception filter function.
func RegisterFrameException(isException func(f runtime.Frame) bool) func() {
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

var _ = RegisterFrameException(func(f runtime.Frame) bool {
	return strings.Contains(f.Function, "runtimekit.")
})

func isException(frame runtime.Frame) bool {
	for _, isException := range exceptions {
		if isException(frame) {
			return true
		}
	}
	return false
}
