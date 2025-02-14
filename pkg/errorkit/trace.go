package errorkit

import (
	"errors"
	"fmt"
	"strings"
)

func WithTrace(err error) error {
	if errors.As(err, &Traced{}) {
		return err
	}

	return Traced{
		Err:   err,
		Trace: nil,
	}
}

type Traced struct {
	Err   error
	Trace []string
}

func (err Traced) Error() string {
	var msg string
	if err.Err != nil {
		msg += err.Error()
	}
	if 0 < len(err.Trace) {
		msg += fmt.Sprintf("\n\n%s", strings.Join(err.Trace, "\n"))
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
