package iterators

func Offset[V any](iter Iterator[V], offset int) Iterator[V] {
	return &offsetIter[V]{
		Iterator: iter,
		Offset:   offset,
	}
}

type offsetIter[V any] struct {
	Iterator[V]
	Offset  int
	skipped int
}

func (oi *offsetIter[V]) Next() bool {
	for oi.skipped < oi.Offset {
		oi.Iterator.Next()
		oi.skipped++
	}
	return oi.Iterator.Next()
}
