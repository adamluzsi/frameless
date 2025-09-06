package slicekit

import (
	"iter"
	"sort"

	"go.llib.dev/frameless/pkg/must"
)

// Map will do a mapping from an input type into an output type.
func Map[O, I any](s []I, mapper func(I) O) []O {
	return must.Must(MapErr[O, I](s, func(i I) (O, error) {
		return mapper(i), nil
	}))
}

// MapErr will do a mapping from an input type into an output type.
func MapErr[O, I any](s []I, mapper func(I) (O, error)) ([]O, error) {
	if s == nil {
		return nil, nil
	}
	var out = make([]O, len(s))
	for index, v := range s {
		o, err := mapper(v)
		if err != nil {
			return out, err
		}
		out[index] = o
	}
	return out, nil
}

// Reduce iterates over a slice, combining elements using the reducer function.
func Reduce[O, I any](s []I, initial O, reducer func(O, I) O) O {
	return must.Must(ReduceErr(s, initial, func(o O, i I) (O, error) {
		return reducer(o, i), nil
	}))
}

// ReduceErr iterates over a slice, combining elements using the reducer function.
func ReduceErr[O, I any](s []I, initial O, reducer func(O, I) (O, error)) (O, error) {
	var result = initial
	for _, i := range s {
		o, err := reducer(result, i)
		if err != nil {
			return result, err
		}
		result = o
	}
	return result, nil
}

func Filter[S ~[]T, T any](s S, filter func(v T) bool) S {
	return must.Must(FilterErr(s, func(v T) (bool, error) {
		return filter(v), nil
	}))
}

func FilterErr[S ~[]T, T any](s S, filter func(v T) (bool, error)) (S, error) {
	if s == nil {
		return nil, nil
	}
	var out = make([]T, 0, len(s))
	for _, val := range s {
		ok, err := filter(val)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, val)
		}
	}
	return out, nil
}

func Lookup[T any](vs []T, index int) (T, bool) {
	index, ok := ResolveIndex(len(vs), index)
	if !ok {
		var zero T
		return zero, false
	}
	return vs[index], true
}

// Merge will merge every []T slice into a single one.
func Merge[S ~[]T, T any](slices ...S) S {
	var out S
	for _, slice := range slices {
		out = append(out, slice...)
	}
	return out
}

// Clone creates a clone from passed src slice.
func Clone[S ~[]T, T any](src S) S {
	if src == nil {
		return nil
	}
	var dst = make(S, len(src))
	copy(dst, src)
	return dst
}

func Batch[T any](vs []T, size int) [][]T {
	var out [][]T
	for i := 0; i < len(vs); i += size {
		end := i + size
		if !(end < len(vs)) {
			end = len(vs)
		}
		out = append(out, vs[i:end])
	}
	return out
}

func Unique[T comparable](vs []T) []T {
	return UniqueBy(vs, func(v T) T { return v })
}

func UniqueBy[T any, ID comparable](vs []T, by func(T) ID) []T {
	var set = make(map[ID]struct{}, len(vs))
	var out []T
	for _, v := range vs {
		id := by(v)
		if _, ok := set[id]; ok {
			continue
		}
		set[id] = struct{}{}
		out = append(out, v)
	}
	return out
}

func Pop[S ~[]T, T any](vs *S) (T, bool) {
	var v T
	if vs == nil {
		return v, false
	}
	if *vs == nil {
		return v, false
	}
	if len(*vs) == 0 {
		return v, false
	}
	i := len(*vs) - 1
	v = (*vs)[i]
	*vs = (*vs)[0:i]
	return v, true
}

func PopAt[S ~[]T, T any](vs *S, index int) (T, bool) {
	if vs == nil {
		var zero T
		return zero, false
	}

	length := len(*vs)

	if length == 0 {
		var zero T
		return zero, false
	}

	index, ok := ResolveIndex(length, index)
	if !ok {
		var zero T
		return zero, false
	}

	val := (*vs)[index]

	*vs = append((*vs)[:index], (*vs)[index+1:]...)
	return val, true
}

func Shift[S ~[]T, T any](vs *S) (T, bool) {
	var v T
	if vs == nil {
		return v, false
	}
	if *vs == nil {
		return v, false
	}
	if len(*vs) == 0 {
		return v, false
	}
	v = (*vs)[0]
	*vs = (*vs)[1:]
	return v, true
}

func Unshift[S ~[]T, T any](vs *S, nvs ...T) {
	if len(nvs) == 0 {
		return
	}
	*vs = append(nvs, *vs...)
}

func Insert[S ~[]T, T any](vs *S, index int, nvs ...T) bool {
	if len(nvs) == 0 {
		return true
	}
	index, ok := ResolveIndex(len(*vs), index)
	if !ok { // out of bound
		if nextindex := len(*vs); index == nextindex {
			*vs = append(*vs, nvs...)
			return true
		}
		return false
	}
	if len(*vs) < index {
		return false
	}
	var og = Clone(*vs)
	*vs = make([]T, 0, len(og)+len(nvs))
	*vs = append(*vs, og[:index]...)
	*vs = append(*vs, nvs...)
	*vs = append(*vs, og[index:]...)
	return true
}

func First[T any](vs []T) (T, bool) {
	if len(vs) == 0 {
		var zero T
		return zero, false
	}
	return vs[0], true
}

func Last[T any](vs []T) (T, bool) {
	if len(vs) == 0 {
		var zero T
		return zero, false
	}
	return vs[len(vs)-1], true
}

func AnyOf[T any](vs []T, filter func(T) bool) bool {
	_, ok := Find(vs, filter)
	return ok
}

// Find will find the first matching element in a slice based on the "by" filter function.
func Find[T any](vs []T, by func(T) bool) (T, bool) {
	for _, v := range vs {
		if by(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// GroupBy will group values in []T based on the group indetifier function.
func GroupBy[S ~[]T, ID comparable, T any](vs S, by func(v T) ID) map[ID]S {
	if len(vs) == 0 {
		return nil
	}
	if by == nil {
		panic("Incorrect use of slicekit.GroupBy[T, ID], it must receive the `func(v T) ID` function!")
	}
	var groups = map[ID]S{}
	for _, v := range vs {
		var id = by(v)
		groups[id] = append(groups[id], v)
	}
	return groups
}

func SortBy[T any](vs []T, less func(a, b T) bool) {
	sort.Slice(vs, func(i, j int) bool {
		return less(vs[i], vs[j])
	})
}

func IterReverse[T any](vs []T) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := len(vs) - 1; i >= 0; i-- {
			if !yield(i, vs[i]) {
				return
			}
		}
	}
}

func Set[T any](vs []T, index int, v T) bool {
	index, ok := ResolveIndex(len(vs), index)
	if !ok {
		return false
	}
	vs[index] = v
	return true
}

func Delete[S ~[]T, T any](vs *S, index int) bool {
	index, ok := ResolveIndex(len(*vs), index)
	if !ok {
		return false
	}
	var out = make(S, 0, len(*vs)-1)
	out = append(out, (*vs)[:index]...)
	out = append(out, (*vs)[index+1:]...)
	*vs = out
	return true
}

// ResolveIndex returns the zero-based element position for index (negative wraps from end),
// It returns false for second argument if the index is out of bounds.
func ResolveIndex(length, index int) (int, bool) {
	if index < 0 {
		n := length + index
		if 0 <= n {
			return n, true
		}
		return index, false
	}
	return index, index < length
}
