package iterators

func Limit[V any](iter Iterator[V], n int) Iterator[V] {
	return &limitIter[V]{
		Iterator: iter,
		Limit:    n,
	}
}

type limitIter[V any] struct {
	Iterator[V]
	Limit int
	index int
}

func (li *limitIter[V]) Next() bool {
	if !li.Iterator.Next() {
		return false
	}
	if !(li.index < li.Limit) {
		return false
	}
	li.index++
	return true
}
