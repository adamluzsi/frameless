// Package mapkit provides utilities working with maps.
//
// The mapkit package is considered as a `lite` pacakge,
// and therefore its dependencies stritcly restricted.
package mapkit

import "go.llib.dev/frameless/pkg/must"

func Map[
	OK comparable, OV any,
	IK comparable, IV any,
](m map[IK]IV, mapper func(IK, IV) (OK, OV)) map[OK]OV {
	return must.Must(MapErr[OK, OV](m, func(ik IK, iv IV) (OK, OV, error) {
		ok, ov := mapper(ik, iv)
		return ok, ov, nil
	}))
}

// MapErr will do a mapping from an input type into an output type.
func MapErr[
	OK comparable, OV any,
	IK comparable, IV any,
](m map[IK]IV, mapper func(IK, IV) (OK, OV, error)) (map[OK]OV, error) {
	if m == nil {
		return nil, nil
	}
	var out = make(map[OK]OV)
	for ik, iv := range m {
		ok, ov, err := mapper(ik, iv)
		if err != nil {
			return out, err
		}
		out[ok] = ov
	}
	return out, nil
}

func Reduce[O any, K comparable, V any](m map[K]V, initial O, reducer func(O, K, V) O) O {
	return must.Must(ReduceErr(m, initial, func(o O, k K, v V) (O, error) {
		return reducer(o, k, v), nil
	}))
}

// ReduceErr goes through the map value, combining elements using the reducer function.
func ReduceErr[O any, K comparable, V any](m map[K]V, initial O, reducer func(O, K, V) (O, error)) (O, error) {
	var result = initial
	for k, v := range m {
		o, err := reducer(result, k, v)
		if err != nil {
			return result, err
		}
		result = o
	}
	return result, nil
}

func Keys[K comparable, V any](m map[K]V, sort ...func([]K)) []K {
	var ks []K
	for k := range m {
		ks = append(ks, k)
	}
	for _, sort := range sort {
		sort(ks)
	}
	return ks
}

func Values[K comparable, V any](m map[K]V, sort ...func([]V)) []V {
	var vs []V
	for _, v := range m {
		vs = append(vs, v)
	}
	for _, sort := range sort {
		sort(vs)
	}
	return vs
}

// Entry is element of a map.
//
// A map is an unordered group of entries,
// where each entry consists of a key and a value.
type Entry[K comparable, V any] struct {
	Key   K
	Value V
}

// ToSlice turns map into map entries.
func ToSlice[K comparable, V any](m map[K]V) []Entry[K, V] {
	if m == nil {
		return nil
	}
	if len(m) == 0 {
		return []Entry[K, V]{}
	}
	var entries []Entry[K, V]
	for k, v := range m {
		entries = append(entries, Entry[K, V]{Key: k, Value: v})
	}
	return entries
}

// Merge will merge all passed map[K]V into a single map[K]V.
// Merging is intentionally order dependent by how the map argument values are passed to Merge.
func Merge[K comparable, V any](maps ...map[K]V) map[K]V {
	var out = make(map[K]V)
	for _, kvs := range maps {
		for k, v := range kvs {
			out[k] = v
		}
	}
	return out
}

// Clone creates a clone from a passed source map.
func Clone[K comparable, V any](m map[K]V) map[K]V {
	var out = make(map[K]V)
	for k, v := range m {
		out[k] = v
	}
	return out
}

func Filter[K comparable, V any](m map[K]V, filter func(k K, v V) bool) map[K]V {
	return must.Must(FilterErr[K, V](m, func(k K, v V) (bool, error) {
		return filter(k, v), nil
	}))
}

func FilterErr[K comparable, V any](m map[K]V, filter func(k K, v V) (bool, error)) (map[K]V, error) {
	var out = make(map[K]V)
	for k, v := range m {
		ok, err := filter(k, v)
		if err != nil {
			return nil, err
		}
		if ok {
			out[k] = v
		}
	}
	return out, nil
}

func Lookup[K comparable, V any](m map[K]V, k K) (V, bool) {
	if m == nil {
		var zero V
		return zero, false
	}
	v, ok := m[k]
	return v, ok
}
