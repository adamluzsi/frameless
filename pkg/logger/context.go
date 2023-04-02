package logger

import (
	"context"
)

type ctxKeyDetails struct{}

type ctxValue struct {
	Super    *ctxValue
	LogEntry logEntry
}

func ContextWith(ctx context.Context, lds ...LoggingDetail) context.Context {
	if len(lds) == 0 {
		return ctx
	}
	var v ctxValue
	if prev, ok := lookupValue(ctx); ok {
		v.Super = prev
	}
	v.LogEntry = make(logEntry)
	for _, ld := range lds {
		ld.addTo(v.LogEntry)
	}
	return context.WithValue(ctx, ctxKeyDetails{}, &v)
}

// getLoggingDetailsFromContext returns the details attached to the context
func getLoggingDetailsFromContext(ctx context.Context) logEntry {
	d := make(logEntry)
	if ctx == nil {
		return d
	}
	if v, ok := lookupValue(ctx); ok {
		for {
			d.Merge(v.LogEntry)
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
