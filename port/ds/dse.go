// Package ds contains common interfaces when we wish to express datastruct behaviours
package ds

import (
	"context"
	"iter"
)

type LenE interface {
	Len(ctx context.Context) (int, error)
}

type ReadOnlyMapE[K comparable, V any] interface {
	Lookup(ctx context.Context, key K) (V, bool, error)
	Get(ctx context.Context, key K) (V, error)
	KeysE[K]
}

type MapE[K comparable, V any] interface {
	ReadOnlyMapE[K, V]
	Set(ctx context.Context, key K, val V) error
	Delete(ctx context.Context, key K) error
}

type KeysE[K any] interface {
	Keys(ctx context.Context) iter.Seq2[K, error]
}

type ValuesE[V any] interface {
	Values(ctx context.Context) iter.Seq2[V, error]
}

type AllE[K, V any, KV KeyValuePair[K, V]] interface {
	All(ctx context.Context) iter.Seq2[KV, error]
}

type KeyValuePair[K, V any] interface {
	Key() K
	Value() V
}

type MapConvertibleE[K comparable, V any] interface {
	ToMap(ctx context.Context) (map[K]V, error)
}

type KV[K, V any] struct {
	K K
	V V
}

var _ KeyValuePair[string, int] = KV[string, int]{}

func (kv KV[K, V]) Key() K   { return kv.K }
func (kv KV[K, V]) Value() V { return kv.V }
