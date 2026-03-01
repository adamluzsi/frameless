// Package ds is a utility function for working with the datastruct.Map port
package dsmap

import (
	"iter"

	"go.llib.dev/frameless/port/ds"
)

func Keys[KVS ds.ReadOnlyMap[K, V], K comparable, V any](kvs KVS) iter.Seq[K] {
	if kvk, ok := any(kvs).(ds.Keys[K]); ok {
		return kvk.Keys()
	}
	return func(yield func(K) bool) {
		for k, _ := range kvs.All() {
			if !yield(k) {
				return
			}
		}
	}
}

func Len[Map ds.ReadOnlyMap[K, V], K comparable, V any](m Map) int {
	switch kvst := any(m).(type) {
	case ds.Len:
		return kvst.Len()
	case ds.Keys[K]:
		var n int
		for range kvst.Keys() {
			n++
		}
		return n
	default:
		var n int
		for range m.All() {
			n++
		}
		return n
	}
}

type Map[K comparable, V any] map[K]V

var _ ds.Map[string, int] = (*Map[string, int])(nil)
var _ ds.ReadOnlyMap[string, int] = (Map[string, int])(nil)
var _ ds.Keys[string] = (Map[string, int])(nil)
var _ ds.Values[int] = (Map[string, int])(nil)
var _ ds.All[string, int] = (Map[string, int])(nil)
var _ ds.MapConveratble[string, int] = (Map[string, int])(nil)

func (m Map[K, V]) Lookup(key K) (V, bool) {
	val, ok := m[key]
	return val, ok
}

func (m Map[K, V]) Get(key K) V { return m[key] }

func (m *Map[K, V]) Set(key K, val V) {
	if *m == nil {
		*m = make(Map[K, V])
	}
	(*m)[key] = val
}

func (m Map[K, V]) Delete(key K) { delete(m, key) }

func (m Map[K, V]) Len() int { return len(m) }

func (m Map[K, V]) ToMap() map[K]V { return m }

func (m Map[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for k, _ := range m {
			if !yield(k) {
				return
			}
		}
	}
}

func (m Map[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

func (m Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k, v := range m {
			if !yield(k, v) {
				return
			}
		}
	}
}
