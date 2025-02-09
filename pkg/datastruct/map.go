package datastruct

type MapInterface[K comparable, V any] interface {
	Lookup(key K) (V, bool)
	Get(key K) V
	Set(key K, val V)
	Delete(key K)

	Len() int
	Keys() []K
}

type Map[K comparable, V any] map[K]V

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

func MapAdd[K comparable, V any, Map MapInterface[K, V]](m Map, k K, v V) func() {
	og, ok := m.Lookup(k)
	m.Set(k, v)
	return func() {
		if ok {
			m.Set(k, og)
		} else {
			m.Delete(k)
		}
	}
}
