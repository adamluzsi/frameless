package logger

import (
	"context"
)

type ctxKeyDetails struct{}

type ctxValue struct {
	Super   *ctxValue
	Details []LoggingDetail
}

func ContextWith(ctx context.Context, lds ...LoggingDetail) context.Context {
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
func getLoggingDetailsFromContext(ctx context.Context, l *Logger) logEntry {
	if ctx == nil {
		return logEntry{}
	}
	var details []LoggingDetail
	if v, ok := lookupValue(ctx); ok {
		for {
			details = append(append([]LoggingDetail{}, v.Details...), details...) // unshift
			if v.Super == nil {
				break
			}
			v = v.Super
		}
	}
	le := make(logEntry)
	if ctx == nil {
		return le
	}
	for _, d := range details {
		d.addTo(l, le)
	}
	return le
}

func lookupValue(ctx context.Context) (*ctxValue, bool) {
	if ptr, ok := ctx.Value(ctxKeyDetails{}).(*ctxValue); ok {
		return ptr, true
	}
	return nil, false
}
