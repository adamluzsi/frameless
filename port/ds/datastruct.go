// Package ds contains common interfaces when we wish to express datastruct behaviours
package ds

import "iter"

type ReadOnlyMap[K comparable, V any] interface {
	Lookup(key K) (V, bool)
	Get(key K) V
	All[K, V]
}

type Map[K comparable, V any] interface {
	ReadOnlyMap[K, V]
	Set(key K, val V)
	Delete(key K)
}

type ReadOnlyList[T any] interface {
	Values[T]
}

type List[T any] interface {
	ReadOnlyList[T]
	Appendable[T]
}

type ReadOnlySequence[T any] interface {
	ReadOnlyList[T]
	Lookup(index int) (T, bool)
}

type Sequence[T any] interface {
	ReadOnlySequence[T]
	List[T]
	Set(index int, val T) bool
	Insert(index int, vs ...T) bool
	Delete(index int) bool
}

type Len interface {
	Len() int
}

type Appendable[T any] interface {
	Append(vs ...T)
}

type Containable[T any] interface {
	Contains(element T) bool
}

type Values[T any] interface {
	Values() iter.Seq[T]
}

type Keys[K any] interface {
	Keys() iter.Seq[K]
}

type All[K, V any] interface {
	All() iter.Seq2[K, V]
}

type MapConveratble[K comparable, V any] interface {
	ToMap() map[K]V
}

type SliceConveratble[T any] interface {
	ToSlice() []T
}
