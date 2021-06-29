package frameless

import "context"

type MetaAccessor interface {
	SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error)
	LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error)
}
