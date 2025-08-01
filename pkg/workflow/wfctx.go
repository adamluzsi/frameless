package workflow

import (
	"context"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/mapkit"
)

var ctxConfigH contextkit.ValueHandler[ctxConfigHKey, ctxConfig]

type ctxConfigHKey struct{}

type ctxConfig struct {
	Participants Participants
	FuncMap      TemplateFuncMap
}

func ContextWithParticipants(ctx context.Context, ps Participants) context.Context {
	if len(ps) == 0 {
		return ctx
	}
	c, _ := ctxConfigH.Lookup(ctx)
	c.Participants = mapkit.Merge(c.Participants, ps)
	return ctxConfigH.ContextWith(ctx, c)
}

func ContextWithFuncMap(ctx context.Context, fm TemplateFuncMap) context.Context {
	if len(fm) == 0 {
		return ctx
	}
	c, _ := ctxConfigH.Lookup(ctx)
	c.FuncMap = mapkit.Merge(c.FuncMap, fm)
	return ctxConfigH.ContextWith(ctx, c)
}
