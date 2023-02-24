package logger

import (
	"context"
)

type Details map[string]any

func (f Details) Merge(oth Details) {
	for k, v := range oth {
		f[k] = v
	}
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
