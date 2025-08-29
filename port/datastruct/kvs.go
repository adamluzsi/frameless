package datastruct

import "iter"

type Map[K comparable, V any] map[K]V

var _ KeyValueStore[any, any] = (Map[any, any])(nil)

func (m Map[K, V]) Lookup(key K) (V, bool) {
	val, ok := m[key]
	return val, ok
}

func (m Map[K, V]) Get(key K) V {
	return m[key]
}

func (m Map[K, V]) Set(key K, val V) { m[key] = val }

func (m Map[K, V]) Delete(key K) { delete(m, key) }

func (m Map[K, V]) Len() int { return len(m) }

func (m Map[K, V]) Keys() []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (m Map[K, V]) Map() map[K]V {
	return m
}

func (m Map[K, V]) Iter() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k, v := range m {
			if !yield(k, v) {
				return
			}
		}
	}
}
