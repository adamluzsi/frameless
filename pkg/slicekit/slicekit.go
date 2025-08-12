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

func Filter[T any](src []T, filter func(v T) bool) []T {
	return must.Must(FilterErr(src, func(v T) (bool, error) {
		return filter(v), nil
	}))
}

func FilterErr[T any](src []T, filter func(v T) (bool, error)) ([]T, error) {
	if src == nil {
		return nil, nil
	}
	var out = make([]T, 0, len(src))
	for _, val := range src {
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
	index, ok := normaliseIndex(len(vs), index)
	if !ok {
		var zero T
		return zero, false
	}
	return vs[index], true
}

// Merge will merge every []T slice into a single one.
func Merge[T any](slices ...[]T) []T {
	var out []T
	for _, slice := range slices {
		out = append(out, slice...)
	}
	return out
}

// Clone creates a clone from passed src slice.
func Clone[T any](src []T) []T {
	if src == nil {
		return nil
	}
	var dst = make([]T, len(src))
	copy(dst, src)
	return dst
}

// Contains reports if a slice contains a given value.
func Contains[T comparable](s []T, v T) bool {
	for _, got := range s {
		if got == v {
			return true
		}
	}
	return false
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

func Pop[T any](vs *[]T) (T, bool) {
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

func PopAt[T any](vs *[]T, index int) (T, bool) {
	if vs == nil {
		var zero T
		return zero, false
	}

	length := len(*vs)

	if length == 0 {
		var zero T
		return zero, false
	}

	index, ok := normaliseIndex(length, index)
	if !ok {
		var zero T
		return zero, false
	}

	val := (*vs)[index]

	*vs = append((*vs)[:index], (*vs)[index+1:]...)
	return val, true
}

func Shift[T any](vs *[]T) (T, bool) {
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

func Unshift[T any](vs *[]T, nvs ...T) {
	if len(nvs) == 0 {
		return
	}
	*vs = append(nvs, *vs...)
}

func Insert[T any](vs *[]T, index int, nvs ...T) {
	if len(nvs) == 0 {
		return
	}
	if len(*vs) < index {
		*vs = append(*vs, nvs...)
		return
	}
	index, ok := normaliseIndex(len(*vs), index)
	if !ok {
		*vs = append(*vs, nvs...)
		return
	} else if len(*vs) < index {
		index = len(*vs)
	}
	var og = Clone(*vs)
	*vs = make([]T, 0, len(og)+len(nvs))
	*vs = append(*vs, og[:index]...)
	*vs = append(*vs, nvs...)
	*vs = append(*vs, og[index:]...)
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
	_, ok := Find[T](vs, filter)
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
func GroupBy[T any, ID comparable](vs []T, by func(v T) ID) map[ID][]T {
	if len(vs) == 0 {
		return nil
	}
	if by == nil {
		panic("Incorrect use of slicekit.GroupBy[T, ID], it must receive the `func(v T) ID` function!")
	}
	var groups = map[ID][]T{}
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

func normaliseIndex(length, index int) (int, bool) {
	if index < 0 {
		n := length + index
		return n, 0 <= n
	}
	return index, index < length
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
