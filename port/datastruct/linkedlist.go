package datastruct

import (
	"iter"

	"go.llib.dev/frameless/pkg/slicekit"
)

type LinkedList[T any] struct {
	head   *llElem[T]
	tail   *llElem[T]
	length int
}

var _ List[any] = (*LinkedList[any])(nil)

type llElem[T any] struct {
	data T
	prev *llElem[T]
	next *llElem[T]
}

func (ll *LinkedList[T]) Iter() iter.Seq[T] {
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

func (ll *LinkedList[T]) Slice() []T {
	var vs []T
	for v := range ll.Iter() {
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
	for v := range ll.Iter() {
		if i == index {
			return v, true
		}
		i++
	}
	var zero T
	return zero, false
}
