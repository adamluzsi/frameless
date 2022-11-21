package errorutil

import (
	"context"
	"errors"
	"fmt"
)

type With struct {
	Err error
}

func (w With) Error() string { return w.Err.Error() }
func (w With) Unwrap() error { return w.Err }

func LookupContext(err error) (context.Context, bool) {
	var detail errorWithContext
	if errors.As(err, &detail) {
		return detail.Ctx, true
	}
	return nil, false
}

// Context will combine an error with a context, so the current context can be used at the place of error handling.
// This can be useful if tracing ID and other helpful values are kept in the context.
func (w With) Context(ctx context.Context) With {
	return With{Err: errorWithContext{
		Err: w.Err,
		Ctx: ctx,
	}}
}

type errorWithContext struct {
	Err error
	Ctx context.Context
}

func (err errorWithContext) Error() string {
	return err.Err.Error()
}

func (err errorWithContext) Unwrap() error {
	return err.Err
}

func LookupDetail(err error) (string, bool) {
	var detail errorWithDetail
	if errors.As(err, &detail) {
		return detail.Detail, true
	}
	return "", false
}

// Detail will return an error that has explanation as detail attached to it.
func (w With) Detail(detail string) With {
	return With{Err: errorWithDetail{
		Err:    w.Err,
		Detail: detail,
	}}
}

// Detailf will return an error that has explanation as detail attached to it.
// Detailf formats according to a fmt format specifier and returns the resulting string.
func (w With) Detailf(format string, a ...any) error {
	return With{Err: errorWithDetail{
		Err:    w.Err,
		Detail: fmt.Sprintf(format, a...),
	}}
}

type errorWithDetail struct {
	Err    error
	Detail string
}

func (err errorWithDetail) Error() string {
	return fmt.Sprintf("%s\n%s", err.Err.Error(), err.Detail)
}

func (err errorWithDetail) Unwrap() error {
	return err.Err
}
