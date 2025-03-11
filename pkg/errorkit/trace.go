package errorkit

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"go.llib.dev/frameless/pkg/runtimekit"
)

func WithTrace(err error) error {
	if err == nil {
		return err
	}
	if errors.As(err, &TracedError{}) {
		return err
	}
	return TracedError{
		Err:   err,
		Stack: runtimekit.Stack(),
	}
}

type TracedError struct {
	Err   error
	Stack []runtime.Frame
}

func (err TracedError) Error() string {
	var msg string
	if err.Err != nil {
		msg += err.Err.Error()
	}
	if 0 < len(err.Stack) {
		var traceLines []string
		for _, frame := range err.Stack {
			traceLines = append(traceLines, err.frameToString(frame))
		}
		if err.Err != nil {
			msg += "\n\n"
		}
		msg += strings.Join(traceLines, "\n")
	}
	return msg
}

func (err TracedError) frameToString(f runtime.Frame) string {
	var msg string
	msg += fmt.Sprintf("%s:%d", f.File, f.Line)
	if 0 < len(f.Function) {
		msg = fmt.Sprintf("%s\n\t%s", f.Function, msg)
	}
	return msg
}

func (err TracedError) As(target any) bool {
	return errors.As(err.Err, target)
}

func (err TracedError) Is(target error) bool {
	return errors.Is(err.Err, target)
}

func (err TracedError) Unwrap() error {
	return err.Err
}

var _ = runtimekit.RegisterTraceException(func(f runtime.Frame) bool {
	return strings.Contains(f.Function, "errorkit.")
})
