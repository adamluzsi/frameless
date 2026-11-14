package errorkit

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"go.llib.dev/frameless/pkg/runtimekit"
)

func WithoutTrace(err error) error {
	if te, ok := As[*TracedError](err); ok {
		te.HideStack = true
		return err
	}
	return err
}

func WithTrace(err error) error {
	if err == nil {
		return err
	}
	var p *TracedError
	if errors.As(err, &p) {
		p.HideStack = false
		return err
	}
	p = &TracedError{
		Err:   err,
		Stack: runtimekit.Stack(),
	}
	p.p = p
	return p
}

type TracedError struct {
	Err   error
	Stack []runtime.Frame

	HideStack bool

	p *TracedError
}

func (err TracedError) Error() string {
	var msg string
	if err.Err != nil {
		msg += err.Err.Error()
	}
	if !err.HideStack && 0 < len(err.Stack) {
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
	switch target := target.(type) {
	case **TracedError:
		var p = err.p
		if p == nil {
			p = &err
		}
		*target = p
		return true
	case *TracedError:
		*target = err
		return true
	default:
		return errors.As(err.Err, target)
	}
}

func (err TracedError) Is(target error) bool {
	return errors.Is(err.Err, target)
}

func (err TracedError) Unwrap() error {
	return err.Err
}

var _ = runtimekit.RegisterFrameException(func(f runtime.Frame) bool {
	return strings.Contains(f.Function, "errorkit.") ||
		strings.Contains(f.Function, "runtime.") ||
		strings.Contains(f.Function, "testing.")
})
