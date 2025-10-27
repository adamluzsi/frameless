package datastruct

import (
	"iter"
)

type List[T any] interface {
	Appendable[T]
	Iterable[T]
	Sizer
}

type Sequence[T any] interface {
	List[T]
	Lookup(index int) (T, bool)
	Set(index int, val T) bool
	Insert(index int, vs ...T) bool
	Delete(index int) bool
}

type KeyValueStore[K comparable, V any] interface {
	Lookup(key K) (V, bool)
	Get(key K) V
	Set(key K, val V)
	Delete(key K)
	Keys() []K
	Iter() iter.Seq2[K, V]
	Sizer
}

type Sizer interface {
	Len() int
}

type Mapper[K comparable, V any] interface {
	// Map returns the contents as a map[K]V.
	Map() map[K]V
}

type Slicer[T any] interface {
	// Slice returns the contents as a slice of T.
	Slice() []T
}

type Iterable[T any] interface {
	Iter() iter.Seq[T]
}

type Appendable[T any] interface {
	Append(vs ...T)
}

type Containable[T any] interface {
	Contains(element T) bool
}
