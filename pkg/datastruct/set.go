package datastruct

import (
	"iter"

	"go.llib.dev/frameless/pkg/slicekit"
)

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

func (s *OrderedSet[T]) Lookup(index int) (T, bool) {
	index, ok := slicekit.ResolveIndex(len(s.vs), index)
	if !ok {
		var zero T
		return zero, false
	}
	for v, i := range s.vs {
		if i == index {
			return v, true
		}
	}
	var zero T
	return zero, false
}

func (s *OrderedSet[T]) Set(index int, v T) bool {
	cur, ok := s.Lookup(index)
	if !ok {
		return false
	}
	delete(s.vs, cur)
	s.vs[v] = index
	return true
}

func (s *OrderedSet[T]) Delete(index int) bool {
	if s.vs == nil {
		return false
	}
	if !(index < len(s.vs)) {
		return false
	}

	var ok bool
	var upd []T
	for v, i := range s.vs {
		if i == index {
			ok = true
			delete(s.vs, v)
			break
		}
		if index < i {
			upd = append(upd, v)
		}
	}
	if !ok {
		return false
	}
	for _, v := range upd {
		s.vs[v]--
	}
	return true
}

func (s *OrderedSet[T]) Insert(index int, vs ...T) bool {
	if s.vs == nil {
		return false
	}
	if !(index < len(s.vs)) {
		return false
	}

	var ok bool
	var upd []T
	for v, i := range s.vs {
		if i == index {
			ok = true
			delete(s.vs, v)
			break
		}
		if index < i {
			upd = append(upd, v)
		}
	}
	if !ok {
		return false
	}
	for _, v := range upd {
		s.vs[v]--
	}
	return true
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
