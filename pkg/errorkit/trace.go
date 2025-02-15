package errorkit

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
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

var stackFunctionExceptions = map[string]struct{}{
	"errorkit.": {},
	"runtime.":  {},
	"testing.":  {},
}

var pkgPath string = reflect.TypeOf(Traced{}).PkgPath()

func getStack() []StackFrame {

	programCounters := make([]uintptr, 1024)
	n_callers := runtime.Callers(1, programCounters)
	frames := runtime.CallersFrames(programCounters[:n_callers])
	var vs []StackFrame

scan:
	for more := true; more; {
		var stackFrameInfo runtime.Frame
		stackFrameInfo, more = frames.Next()

		if strings.Contains(stackFrameInfo.File, pkgPath) {
			continue
		}

		for exception := range stackFunctionExceptions {
			if strings.Contains(stackFrameInfo.Function, exception) {
				continue scan
			}
		}

		vs = append(vs, StackFrame{
			Function: stackFrameInfo.Function,
			File:     stackFrameInfo.File,
			Line:     stackFrameInfo.Line,
		})
	}

	return vs
}
