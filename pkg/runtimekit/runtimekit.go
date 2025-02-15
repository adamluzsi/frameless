package runtimekit

import (
	"runtime"
	"strings"

	"go.llib.dev/frameless/port/option"
)

func Stack(opts ...StackOption) []StackFrame {
	options := option.Use(opts)

	programCounters := make([]uintptr, 1024)
	n_callers := runtime.Callers(1+options.Skip, programCounters)
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

type StackOption interface {
	option.Option[StackOptions]
}

type StackOptions struct {
	Skip       int `default:"0"`
	BufferSize int `default:"1024"`
}

func (r StackOptions) Configure(t *StackOptions) { option.Configure(r, t) }

type StackFrame struct {
	Function string
	File     string
	Line     int
}
