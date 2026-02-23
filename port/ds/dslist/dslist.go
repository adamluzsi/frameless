package dslist

import (
	"iter"
	"slices"

	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/ds"
)

func Len[List ds.ReadOnlyList[T], T any](l List) int {
	switch list := any(l).(type) {
	case ds.Len:
		return list.Len()
	default:
		var n int
		for range l.Values() {
			n++
		}
		return n
	}
}

type Slice[T any] []T

var _ ds.List[any] = (*Slice[any])(nil)
var _ ds.ReadOnlyList[any] = (*Slice[any])(nil)
var _ ds.Len = (*Slice[any])(nil)
var _ ds.Values[string] = (*Slice[string])(nil)
var _ ds.All[int, string] = (*Slice[string])(nil)

func (slc *Slice[T]) Append(vs ...T) {
	*slc = append(*slc, vs...)
}

func (slc Slice[T]) Len() int {
	return len(slc)
}

func (slc Slice[T]) All() iter.Seq2[int, T] {
	return slices.All(slc)
}

func (slc Slice[T]) Values() iter.Seq[T] {
	return slices.Values(slc)
}

///////////////////////////////////////////////////////////////////////////////////////////////////

type LinkedList[T any] struct {
	head   *llElem[T]
	tail   *llElem[T]
	length int
}

var _ ds.List[any] = (*LinkedList[any])(nil)
var _ ds.ReadOnlyList[any] = (*LinkedList[any])(nil)
var _ ds.Len = (*LinkedList[any])(nil)
var _ ds.Values[string] = (*LinkedList[string])(nil)

type llElem[T any] struct {
	data T
	prev *llElem[T]
	next *llElem[T]
}

func (ll *LinkedList[T]) Values() iter.Seq[T] {
	return func(yield func(T) bool) {
		if ll == nil {
			return
		}
		var current = ll.head
		for {
			if current == nil {
				break
			}
			if !yield(current.data) {
				return
			}
			current = current.next
		}
	}
}

func (ll *LinkedList[T]) ToSlice() []T {
	var vs []T
	for v := range ll.Values() {
		vs = append(vs, v)
	}
	return vs
}

func (ll *LinkedList[T]) Append(vs ...T) {
	for _, v := range vs {
		ll.append(v)
	}
}

func (ll *LinkedList[T]) append(v T) {
	newNode := &llElem[T]{data: v}
	if ll.tail == nil {
		ll.head = newNode
		ll.tail = newNode
	} else {
		prevTail := ll.tail
		prevTail.next = newNode
		ll.tail = newNode
		ll.tail.prev = prevTail
	}
	ll.length++
}

// Prepend adds an element to the beginning of the list.
func (ll *LinkedList[T]) Prepend(vs ...T) {
	if len(vs) == 0 {
		return
	}
	for _, v := range slicekit.IterReverse(vs) {
		ll.prepend(v)
	}
}

func (ll *LinkedList[T]) prepend(v T) {
	var (
		prevHead = ll.head
		newHead  = &llElem[T]{
			data: v,
			next: prevHead,
		}
	)
	if prevHead != nil {
		prevHead.prev = newHead
	}
	ll.head = newHead
	if ll.tail == nil {
		ll.tail = newHead
	}
	ll.length++
}

// Len returns the length of elements in the list
func (ll *LinkedList[T]) Len() int {
	return ll.length
}

func (ll *LinkedList[T]) Shift() (T, bool) {
	if ll.head == nil {
		var zero T
		return zero, false
	}
	first := ll.head
	ll.head = first.next
	if ll.head != nil {
		ll.head.prev = nil
	}
	if ll.head == nil {
		ll.tail = nil
	}
	ll.length--
	return first.data, true
}

func (ll *LinkedList[T]) Pop() (T, bool) {
	var last = ll.tail
	if last == nil {
		var zero T
		return zero, false
	}
	var prev = ll.tail.prev
	if prev != nil {
		prev.next = nil
	}
	if prev == nil {
		ll.head = nil
	}
	ll.tail = prev
	ll.length--
	return last.data, true
}

func (ll *LinkedList[T]) Lookup(index int) (T, bool) {
	if index < 0 {
		var zero T
		return zero, false
	}
	var ok = index < ll.length
	if !ok {
		var zero T
		return zero, false
	}
	var i int
	for v := range ll.Values() {
		if i == index {
			return v, true
		}
		i++
	}
	var zero T
	return zero, false
}
