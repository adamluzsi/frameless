package errorkit

import (
	"context"
	"errors"
	"fmt"
)

func With(err error) WithBuilder { return WithBuilder{Err: err} }

type WithBuilder struct{ Err error }

func (w WithBuilder) Error() string { return w.Err.Error() }

func (w WithBuilder) Wrap(err error) WithBuilder {
	return WithBuilder{Err: Merge(w.Err, err)}
}

func (w WithBuilder) Unwrap() error { return w.Err }

func LookupContext(err error) (context.Context, bool) {
	var detail withContext
	if errors.As(err, &detail) {
		return detail.Ctx, true
	}
	return nil, false
}

// Context will combine an error with a context, so the current context can be used at the place of error handling.
// This can be useful if tracing ID and other helpful values are kept in the context.
func (w WithBuilder) Context(ctx context.Context) WithBuilder {
	return WithBuilder{Err: withContext{
		Err: w.Err,
		Ctx: ctx,
	}}
}

type withContext struct {
	Err error
	Ctx context.Context
}

func (err withContext) Error() string {
	return err.Err.Error()
}

func (err withContext) Unwrap() error {
	return err.Err
}

func LookupDetail(err error) (string, bool) {
	var detail withDetail
	if errors.As(err, &detail) {
		return detail.Detail, true
	}
	return "", false
}

// Detail will return an error that has explanation as detail attached to it.
func (w WithBuilder) Detail(detail string) WithBuilder {
	return WithBuilder{Err: withDetail{
		Err:    w.Err,
		Detail: detail,
	}}
}

// Detailf will return an error that has explanation as detail attached to it.
// Detailf formats according to a fmt format specifier and returns the resulting string.
func (w WithBuilder) Detailf(format string, a ...any) WithBuilder {
	return WithBuilder{Err: withDetail{
		Err:    w.Err,
		Detail: fmt.Sprintf(format, a...),
	}}
}

type withDetail struct {
	Err    error
	Detail string
}

func (err withDetail) Error() string {
	return fmt.Sprintf("%s\n%s", err.Err.Error(), err.Detail)
}

func (err withDetail) Unwrap() error {
	return err.Err
}
