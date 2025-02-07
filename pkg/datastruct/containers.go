package datastruct

type Stack[T any] []T

// IsEmpty check if stack is empty
func (s *Stack[T]) IsEmpty() bool {
	return len(*s) == 0
}

// Push a new value onto the stack
func (s *Stack[T]) Push(v T) {
	*s = append(*s, v) // Simply append the new value to the end of the stack
}

// Pop remove and return top element of stack. Return false if stack is empty.
func (s *Stack[T]) Pop() (T, bool) {
	if s.IsEmpty() {
		return *new(T), false
	}
	index := len(*s) - 1   // Get the index of the top most element.
	element := (*s)[index] // Index into the slice and obtain the element.
	*s = (*s)[:index]      // Remove it from the stack by slicing it off.
	return element, true
}

// Last returns the last stack element
func (s *Stack[T]) Last() (T, bool) {
	if s.IsEmpty() {
		return *new(T), false
	}
	return (*s)[(len(*s) - 1)], true
}

func MakeSet[T comparable](s ...T) Set[T] {
	var set Set[T]
	for _, v := range s {
		set.Add(v)
	}
	return set
}

type Set[T comparable] struct {
	vs map[T]struct{}
}

func (s *Set[T]) Add(v T) {
	if s.vs == nil {
		s.vs = make(map[T]struct{})
	}
	s.vs[v] = struct{}{}
}

func (s Set[T]) Has(v T) bool {
	if s.vs == nil {
		return false
	}
	_, ok := s.vs[v]
	return ok
}

func (s Set[T]) ToSlice() []T {
	var out []T
	for v := range s.vs {
		out = append(out, v)
	}
	return out
}
