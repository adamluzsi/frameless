package logging

import (
	"context"
)

type ctxKeyDetails struct{}

type ctxValue struct {
	Super   *ctxValue
	Details []Detail
}

func ContextWith(ctx context.Context, lds ...Detail) context.Context {
	if len(lds) == 0 {
		return ctx
	}
	var v ctxValue
	if prev, ok := lookupValue(ctx); ok {
		v.Super = prev
	}
	v.Details = lds
	return context.WithValue(ctx, ctxKeyDetails{}, &v)
}

// getLoggingDetailsFromContext returns the details attached to the context
func getLoggingDetailsFromContext(ctx context.Context) []Detail {
	if ctx == nil {
		return nil
	}
	var details []Detail
	if v, ok := lookupValue(ctx); ok {
		for {
			details = append(append([]Detail{}, v.Details...), details...) // unshift
			if v.Super == nil {
				break
			}
			v = v.Super
		}
	}
	return details
}

func lookupValue(ctx context.Context) (*ctxValue, bool) {
	if ptr, ok := ctx.Value(ctxKeyDetails{}).(*ctxValue); ok {
		return ptr, true
	}
	return nil, false
}
