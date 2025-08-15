package datastruct

import "iter"

type OrderedSet[T comparable] struct {
	vs map[T]int
}

var _ List[any] = (*OrderedSet[any])(nil)

func (s *OrderedSet[T]) Append(vs ...T) {
	for _, v := range vs {
		s.add(v)
	}
}

func (s *OrderedSet[T]) add(v T) {
	if s.vs == nil {
		s.vs = make(map[T]int)
	}
	if _, ok := s.vs[v]; ok {
		return
	}
	index := len(s.vs)
	s.vs[v] = index
}

func (s OrderedSet[T]) Has(v T) bool {
	if s.vs == nil {
		return false
	}
	_, ok := s.vs[v]
	return ok
}

func (set OrderedSet[T]) FromSlice(vs []T) OrderedSet[T] {
	set.Append(vs...)
	return set
}

func (s OrderedSet[T]) ToSlice() []T {
	var out []T = make([]T, len(s.vs))
	for v, index := range s.vs {
		out[index] = v
	}
	return out
}

func (s OrderedSet[T]) Len() int {
	return len(s.vs)
}

func (s OrderedSet[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		for k := range s.vs {
			if !yield(k) {
				return
			}
		}
	}
}
