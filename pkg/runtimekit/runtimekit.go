package runtimekit

import (
	"fmt"
	"runtime"
	"strings"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/option"
)

// Create a new error with given error or error message.
// TracedErrors extends the error message by a human readable stack trace.
func Stack(opts ...StackOption) string {
	options := option.Use(opts)

	programCounters := make([]uintptr, 1024)

	const skipNewTracedError = 1
	const skipRuntimeCallers = 1
	const skipFrames = skipNewTracedError + skipRuntimeCallers

	n_callers := runtime.Callers(skipFrames, programCounters)
	frames := runtime.CallersFrames(programCounters[:n_callers])

	var vs []TraceFrame
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

		vs = append(vs, TraceFrame{
			Function: stackFrameInfo.Function,
			File:     stackFrameInfo.File,
			Line:     stackFrameInfo.Line,
		})
	}

	return slicekit.Reduce(vs, "", func(o string, f TraceFrame) string {
		return fmt.Sprintf("%s%s\n", o, f.String())
	})
}

type StackOption interface {
	option.Option[StackOptions]
}

type StackOptions struct {
	Offset     int `default:"1"`
	BufferSize int `default:"1024"`
}
