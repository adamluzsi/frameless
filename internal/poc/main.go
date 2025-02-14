package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/pp"
)

type TraceFrame struct {
	Function string
	File     string
	Line     int
}

func (tf TraceFrame) String() string {
	var str string
	str += fmt.Sprintf("%s:%d", tf.File, tf.Line)
	if 0 < len(tf.Function) {
		str = fmt.Sprintf("%s\n\t%s", tf.Function, str)
	}
	return str
}

// Create a new error with given error or error message.
// TracedErrors extends the error message by a human readable stack trace.
func GetStackTraceB(offset int) string {
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

func main() {
	s := reflect.TypeOf(exampleStruct{})
	for i := 0; i < s.NumField(); i++ {
		field := s.Field(i)
		s.PkgPath()
		fmt.Printf("Field %s is from package: %v\n", field.Name, field.PkgPath)
	}
	// Example usage of GetStackTrace
	funcA()
}

// This is a dummy function to show how the stack trace works.
func funcA() {
	funcB()
}

// This is another dummy function to show how the stack trace works.
func funcB() {
	fmt.Println(GetStackTraceB(2))
}

type exampleStruct struct {
	FieldA string // This field belongs to package "main" in this context
}
