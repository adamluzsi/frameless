package httpkit

import (
	"context"
	"regexp"
	"slices"
	"strings"
)

type pathParam struct {
	prev  *pathParam
	Key   string
	Value string
}

func (p pathParam) ToPathParams() map[string]string {
	var pps []pathParam
	pps = append(pps, p)
	for cp := p; cp.prev != nil; cp = *cp.prev {
		pps = append(pps, *cp.prev)
	}
	slices.Reverse(pps)
	var pp = make(map[string]string)
	for _, p := range pps {
		pp[p.Key] = p.Value
	}
	return pp
}

type ctxKeyPathParam struct{}

func WithPathParam(ctx context.Context, key, val string) context.Context {
	var pp = pathParam{
		Key:   key,
		Value: val,
	}
	if prev, ok := lookupPathParam(ctx); ok {
		pp.prev = &prev
	}
	return context.WithValue(ctx, ctxKeyPathParam{}, pp)
}

func PathParams(ctx context.Context) map[string]string {
	var (
		pp        = make(map[string]string)
		pps       []pathParam
		param, ok = lookupPathParam(ctx)
	)
	if !ok {
		return pp
	}
	pps = append(pps, param)
	for cp := param; cp.prev != nil; cp = *cp.prev {
		pps = append(pps, *cp.prev)
	}
	slices.Reverse(pps)
	for _, p := range pps {
		pp[p.Key] = p.Value
	}
	return pp
}

func lookupPathParam(ctx context.Context) (pathParam, bool) {
	if ctx == nil {
		return pathParam{}, false
	}
	pp, ok := ctx.Value(ctxKeyPathParam{}).(pathParam)
	return pp, ok
}

var pathParamPlaceholderRGX = regexp.MustCompile(`^:.+$`)

func isPathParamPlaceholder(pathpart string) (varName string, ok bool) {
	ok = pathParamPlaceholderRGX.MatchString(pathpart)
	if ok {
		varName = strings.TrimPrefix(pathpart, ":")
	}
	return varName, ok
}
