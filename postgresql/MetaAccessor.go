package postgresql

import (
	"context"
	"encoding/json"
)

type MetaAccessor struct{}

type ctxMetaKey struct{}
type metaMap map[string]json.RawMessage

func (ma MetaAccessor) SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error) {
	bs, err := json.Marshal(value)
	if err != nil {
		return ctx, err
	}

	mm, ok := ma.lookupMetaMap(ctx)
	if !ok {
		mm = make(metaMap)
		ctx = ma.setMetaMap(ctx, mm)
	}
	mm[key] = bs

	return ctx, nil
}

func (ma MetaAccessor) LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error) {
	if ctx == nil {
		return false, nil
	}
	mm, ok := ma.lookupMetaMap(ctx)
	if !ok {
		return false, nil
	}
	bs, ok := mm[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(bs, ptr)
}

func (ma MetaAccessor) setMetaMap(ctx context.Context, mm metaMap) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxMetaKey{}, mm)
}

func (ma MetaAccessor) lookupMetaMap(ctx context.Context) (metaMap, bool) {
	if ctx == nil {
		return nil, false
	}
	mm, ok := ctx.Value(ctxMetaKey{}).(metaMap)
	return mm, ok
}
