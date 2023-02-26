package logger

import (
	"context"
	"errors"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/logger/logdto"
)

type Details map[string]any

func (ld Details) Merge(oth Details) Details {
	for k, v := range oth {
		ld[k] = v
	}
	return ld
}

func (ld Details) Err(err error) Details {
	errDetail := logdto.Error{}
	errDetail.Message = err.Error()
	if usrErr := (errorutil.UserError{}); errors.As(err, &usrErr) {
		errDetail.Code = usrErr.ID.String()
		errDetail.Detail = usrErr.Message.String()
	}
	if eud, ok := errorutil.LookupDetail(err); ok {
		errDetail.Detail = eud
	}
	ld["error"] = errDetail
	return ld
}

type ctxKeyDetails struct{}

type ctxValue struct {
	Super   *ctxValue
	Details Details
}

func ContextWithDetails(ctx context.Context, details Details) context.Context {
	if details == nil {
		return ctx
	}
	var v ctxValue
	if prev, ok := lookupValue(ctx); ok {
		v.Super = prev
	}
	v.Details = make(Details)
	v.Details.Merge(details)
	return context.WithValue(ctx, ctxKeyDetails{}, &v)
}

// getDetailsFromContext returns the details attached to the context
func getDetailsFromContext(ctx context.Context) Details {
	d := make(Details)
	if ctx == nil {
		return d
	}
	if v, ok := lookupValue(ctx); ok {
		for {
			d.Merge(v.Details)
			if v.Super == nil {
				break
			}
			v = v.Super
		}
	}
	return d
}

func lookupValue(ctx context.Context) (*ctxValue, bool) {
	if ptr, ok := ctx.Value(ctxKeyDetails{}).(*ctxValue); ok {
		return ptr, true
	}
	return nil, false
}
