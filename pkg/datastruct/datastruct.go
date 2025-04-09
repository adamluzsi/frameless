package datastruct

type Stack[T any] []T

// IsEmpty check if stack is empty
func (s *Stack[T]) IsEmpty() bool {
	return len(*s) == 0
}

// Push a new value onto the stack
func (s *Stack[T]) Push(v T) {
	*s = append(*s, v) // Simply append the new value to the end of the stack
}

// Pop remove and return top element of stack. Return false if stack is empty.
func (s *Stack[T]) Pop() (T, bool) {
	if s.IsEmpty() {
		return *new(T), false
	}
	index := len(*s) - 1   // Get the index of the top most element.
	element := (*s)[index] // Index into the slice and obtain the element.
	*s = (*s)[:index]      // Remove it from the stack by slicing it off.
	return element, true
}

// Last returns the last stack element
func (s *Stack[T]) Last() (T, bool) {
	if s.IsEmpty() {
		return *new(T), false
	}
	return (*s)[(len(*s) - 1)], true
}

type Set[T comparable] struct {
	vs map[T]int
}

func (s *Set[T]) Add(v T) {
	if s.vs == nil {
		s.vs = make(map[T]int)
	}
	if _, ok := s.vs[v]; ok {
		return
	}
	index := len(s.vs)
	s.vs[v] = index
}

func (s Set[T]) Has(v T) bool {
	if s.vs == nil {
		return false
	}
	_, ok := s.vs[v]
	return ok
}

func (set Set[T]) From(vs ...T) Set[T] {
	return set.FromSlice(vs)
}

func (set Set[T]) FromSlice(vs []T) Set[T] {
	for _, v := range vs {
		set.Add(v)
	}
	return set
}

func (s Set[T]) ToSlice() []T {
	var out []T = make([]T, len(s.vs))
	for v, index := range s.vs {
		out[index] = v
	}
	return out
}

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
