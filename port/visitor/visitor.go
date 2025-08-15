package visitor

import (
	"iter"
)

type Visitable[C, T any] interface {
	AcceptVisitor(c C, v Visitor[C, T])
}

type Visitor[C, T any] interface {
	Visit(c C, v T)
}

func Walk[C, T any](c C, v Visitable[C, T]) iter.Seq2[C, T] {
	return func(yield func(C, T) bool) {
		if v == nil {
			return
		}
		w := &walker[C, T]{yield: yield}
		v.AcceptVisitor(c, w)
	}
}

type walker[C, T any] struct {
	yield func(C, T) bool
	done  bool
}

func (w *walker[C, T]) Visit(c C, v T) {
	if w.done {
		return
	}
	w.done = !w.yield(c, v)
}
