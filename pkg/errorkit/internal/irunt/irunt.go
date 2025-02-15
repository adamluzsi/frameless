package irunt

import (
	"runtime"
	"strings"
)

func Stack(opts StackOptions) []StackFrame {
	programCounters := make([]uintptr, 1024)
	n_callers := runtime.Callers(1+opts.Skip, programCounters)
	frames := runtime.CallersFrames(programCounters[:n_callers])

	var vs []StackFrame
	for more := true; more; {
		var stackFrameInfo runtime.Frame
		stackFrameInfo, more = frames.Next()

		if stackFrameInfo.Function == "asciichgolangpublic.TracedErrorf" {
			continue
		}

		if strings.HasPrefix(stackFrameInfo.Function, "errorkit.") {
			continue
		}

		if stackFrameInfo.Function == "testing.tRunner" {
			break
		}

		if stackFrameInfo.Function == "runtime.main" {
			break
		}

		vs = append(vs, StackFrame{
			Function: stackFrameInfo.Function,
			File:     stackFrameInfo.File,
			Line:     stackFrameInfo.Line,
		})
	}

	return vs
}

type StackOptions struct {
	Skip       int
	BufferSize int
}

type StackFrame struct {
	Function string
	File     string
	Line     int
}
