package datastruct

import "iter"

type List[T any] interface {
	Append(vs ...T)
	ToSlice() []T
	Iter() iter.Seq[T]
	Sizer
}

type Sequence[T any] interface {
	List[T]
	Lookup(index int) (T, bool)
	Set(index int, val T) bool
	Insert(index int, vs ...T) bool
	Delete(index int) bool
}

// type Deque[T any] interface {
// 	Shift() (T, bool)
// 	Pop() (T, bool)
// 	Unshift()
// }

// KVS stands for Key Value Store, and a common interface for map[K]V types.
type KVS[K comparable, V any] interface {
	Lookup(key K) (V, bool)
	Get(key K) V
	Set(key K, val V)
	Delete(key K)
	Keys() []K
	ToMap() map[K]V
	Iter() iter.Seq2[K, V]
	Sizer
}

type Sizer interface {
	Len() int
}
