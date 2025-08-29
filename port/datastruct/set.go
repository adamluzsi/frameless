// Package datastruct is an experimental package, that is a candidate to become a port if there is use-case to support it.
package datastruct

import (
	"fmt"
	"iter"
	"slices"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

type Set[T comparable] map[T]struct{}

func (s *Set[T]) Append(vs ...T) {
	if *s == nil {
		*s = make(Set[T])
	}
	for _, v := range vs {
		(*s)[v] = struct{}{}
	}
}

func (s *Set[T]) Slice() []T {
	return iterkit.Collect(s.Iter())
}

func (s *Set[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		for v, _ := range *s {
			if !yield(v) {
				return
			}
		}
	}
}

func (s *Set[T]) Len() int {
	return len(*s)
}

type OrderedSet[T comparable] struct {
	vs map[T]struct{}
	is map[int]*T
}

var _ List[any] = (*OrderedSet[any])(nil)
var _ Sequence[any] = (*OrderedSet[any])(nil)

func (s *OrderedSet[T]) init() {
	if s.vs == nil {
		s.vs = make(map[T]struct{})
	}
	if s.is == nil {
		s.is = make(map[int]*T)
	}
}

func (s *OrderedSet[T]) Append(vs ...T) {
	for _, v := range vs {
		s.add(len(s.vs), v)
	}
}

func (s *OrderedSet[T]) add(index int, v T) {
	s.init()
	if _, ok := s.vs[v]; ok {
		return
	}
	s.vs[v] = struct{}{}
	s.is[index] = &v
	// the cached variable might be an issue as the variable won't be released after the stack,
	// but let's think about this issue later
}

func (s *OrderedSet[T]) Lookup(index int) (T, bool) {
	index, ok := slicekit.ResolveIndex(len(s.vs), index)
	if !ok {
		var zero T
		return zero, false
	}
	if k, ok := s.is[index]; ok {
		return *k, true
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
	delete(s.is, index)
	s.add(index, v)
	return true
}

func (s *OrderedSet[T]) Delete(index int) bool {
	index, ok := slicekit.ResolveIndex(len(s.vs), index)
	if !ok {
		return false
	}
	ptr, ok := s.is[index]
	if !ok {
		return false
	}
	delete(s.is, index)
	delete(s.vs, *ptr)
	s.offsetIndexesBy(index+1, -1)
	return true
}

func (s *OrderedSet[T]) Insert(index int, vs ...T) bool {
	if len(vs) == 0 {
		return true
	}

	s.init()

	index, ok := slicekit.ResolveIndex(len(s.vs), index)
	if !ok && index != len(s.vs) {
		return false
	}

	if _, ok := s.Lookup(index); !ok {
		if index == len(s.vs) { // index point to next index candidate, act as append
			s.Append(vs...)
			return true
		}
		return false
	}

	s.offsetIndexesBy(index, len(vs))

	// add new values
	for i, v := range vs {
		s.add(index+i, v)
	}
	return true
}

// offset existing values to the right to make room for the new values
func (s *OrderedSet[T]) offsetIndexesBy(fromIndex, offset int) {
	var idxs = slicekit.Filter(s.indexes(), func(v int) bool {
		return fromIndex <= v
	})
	if 0 < offset { // offset to the right
		slices.Reverse(idxs)
	}
	for _, currentIndex := range idxs {
		newIndex := currentIndex + offset
		ptr, ok := s.is[currentIndex]
		if !ok {
			panic(fmt.Sprintf("[implementation-error] missing value at index %d from OrderedSet", currentIndex))
		}
		delete(s.is, currentIndex)
		if _, ok := s.is[newIndex]; ok {
			panic(fmt.Sprintf("[implementation-error] in OrderedSet, the targeted new index (%d) during index offsetting were already occupied", currentIndex))
		}
		s.is[newIndex] = ptr
	}
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

func (s OrderedSet[T]) Slice() []T {
	var out []T = make([]T, len(s.is))
	for i, ptr := range s.is {
		out[i] = *ptr
	}
	return out
}

func (s OrderedSet[T]) Len() int {
	return len(s.vs)
}

func (s OrderedSet[T]) indexes() []int {
	indexes := mapkit.Keys(s.is)
	slices.Sort(indexes)
	return indexes
}

func (s OrderedSet[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		for i := 0; i < len(s.is); i++ {
			if !yield(*s.is[i]) {
				return
			}
		}
	}
}
