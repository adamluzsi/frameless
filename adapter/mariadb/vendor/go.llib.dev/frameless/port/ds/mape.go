package ds

import (
	"context"
	"iter"
)

func AsMapE[K comparable, V any](m Map[K, V]) MapE[K, V] {
	return toMapE[K, V]{m: m}
}

type toMapE[K comparable, V any] struct {
	m Map[K, V]
}

var _ MapE[string, int] = toMapE[string, int]{}

func (a toMapE[K, V]) err(ctx context.Context) error {
	if ctx != nil {
		return ctx.Err()
	}
	return nil
}

func (a toMapE[K, V]) Lookup(ctx context.Context, key K) (V, bool, error) {
	v, ok := a.m.Lookup(key)
	return v, ok, a.err(ctx)
}

func (a toMapE[K, V]) Set(ctx context.Context, key K, val V) error {
	a.m.Set(key, val)
	return a.err(ctx)
}

func (a toMapE[K, V]) Get(ctx context.Context, key K) (V, error) {
	return a.m.Get(key), a.err(ctx)
}

func (a toMapE[K, V]) Delete(ctx context.Context, key K) error {
	a.m.Delete(key)
	return a.err(ctx)
}

var _ KeysE[string] = toMapE[string, int]{}

func (a toMapE[K, V]) Keys(ctx context.Context) iter.Seq2[K, error] {
	if ctx == nil {
		ctx = context.Background()
	}
	return func(yield func(K, error) bool) {
		for key := range a.m.Keys() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !yield(key, ctx.Err()) {
				return
			}
		}
	}
}

var _ ValuesE[int] = toMapE[string, int]{}

func (a toMapE[K, V]) Values(ctx context.Context) iter.Seq2[V, error] {
	if ctx == nil {
		ctx = context.Background()
	}
	if vs, ok := (a.m).(Values[V]); ok {
		return func(yield func(V, error) bool) {
			for value := range vs.Values() {
				if !yield(value, a.err(ctx)) {
					return
				}
			}
		}
	}
	return func(yield func(V, error) bool) {
		for key := range a.Keys(ctx) {
			if !yield(a.m.Get(key), a.err(ctx)) {
				return
			}
		}
	}
}

var _ AllE[string, int, KV[string, int]] = toMapE[string, int]{}

func (a toMapE[K, V]) All(ctx context.Context) iter.Seq2[KV[K, V], error] {
	if all, ok := a.m.(All[K, V]); ok {
		return func(yield func(KV[K, V], error) bool) {
			for k, v := range all.All() {
				if !yield(KV[K, V]{K: k, V: v}, a.err(ctx)) {
					return
				}
			}
		}
	}
	return func(yield func(KV[K, V], error) bool) {
		for key := range a.m.Keys() {
			if !yield(KV[K, V]{K: key, V: a.m.Get(key)}, a.err(ctx)) {
				return
			}
		}
	}
}

var _ LenE = toMapE[string, int]{}

func (a toMapE[K, V]) Len(ctx context.Context) (int, error) {
	if l, ok := a.m.(Len); ok {
		return l.Len(), a.err(ctx)
	}
	var n int
	for _, err := range a.Keys(ctx) {
		if err != nil {
			return 0, err
		}
		n++
	}
	return n, a.err(ctx)
}
