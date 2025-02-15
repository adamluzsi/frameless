package errorkit

import (
	"errors"
	"fmt"
	"strings"

	"go.llib.dev/frameless/pkg/errorkit/internal/irunt"
	"go.llib.dev/frameless/pkg/slicekit"
)

func WithTrace(err error) error {
	if errors.As(err, &Traced{}) {
		return err
	}
	return Traced{
		Err: err,
		Trace: slicekit.Map(irunt.Stack(irunt.StackOptions{
			Skip:       1,
			BufferSize: 1024,
		}), func(frame irunt.StackFrame) StackFrame {
			return StackFrame(frame)
		}),
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
		msg += err.Error()
	}
	if 0 < len(err.Trace) {
		msg += fmt.Sprintf("\n\n%s", strings.Join(slicekit.Map(err.Trace, StackFrame.String), "\n"))
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
