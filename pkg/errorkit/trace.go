package errorkit

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
)

func WithTrace(err error) error {
	if err == nil {
		return err
	}
	if errors.As(err, &Traced{}) {
		return err
	}
	return Traced{
		Err:   err,
		Trace: getStack(),
	}
}

type Traced struct {
	Err   error
	Trace []StackFrame
}

type StackFrame struct {
	Function string
	File     string
	Line     int
}

func (f StackFrame) String() string {
	var msg string
	msg += fmt.Sprintf("%s:%d", f.File, f.Line)
	if 0 < len(f.Function) {
		msg = fmt.Sprintf("%s\n\t%s", f.Function, msg)
	}
	return msg
}

func (err Traced) Error() string {
	var msg string
	if err.Err != nil {
		msg += err.Err.Error()
	}
	if 0 < len(err.Trace) {
		var traceLines []string
		for _, frame := range err.Trace {
			traceLines = append(traceLines, frame.String())
		}
		if err.Err != nil {
			msg += "\n\n"
		}
		msg += strings.Join(traceLines, "\n")
	}
	return msg
}

func (err Traced) As(target any) bool {
	return errors.As(err.Err, target)
}

func (err Traced) Is(target error) bool {
	return errors.Is(err.Err, target)
}

func (err Traced) Unwrap() error {
	return err.Err
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
	return strings.Contains(f.Function, "errorkit.")
})

var _ = RegisterTraceException(func(f runtime.Frame) bool {
	return strings.Contains(f.Function, "runtime.") ||
		strings.Contains(f.Function, "testing.")
})

func getStack() []StackFrame {
	programCounters := make([]uintptr, 1024)
	n_callers := runtime.Callers(1, programCounters)
	frames := runtime.CallersFrames(programCounters[:n_callers])
	var vs []StackFrame

tracing:
	for more := true; more; {
		var frame runtime.Frame
		frame, more = frames.Next()

		for _, exception := range exceptions {
			if exception(frame) {
				continue tracing
			}
		}

		vs = append(vs, StackFrame{
			Function: frame.Function,
			File:     frame.File,
			Line:     frame.Line,
		})
	}

	return vs
}
