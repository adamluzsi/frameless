package iterators

import "github.com/adamluzsi/frameless"

func Stub[T any](i frameless.Iterator[T]) *StubIter[T] {
	return &StubIter[T]{
		Iterator:  i,
		StubValue: i.Value,
		StubClose: i.Close,
		StubNext:  i.Next,
		StubErr:   i.Err,
	}
}

type StubIter[T any] struct {
	Iterator  frameless.Iterator[T]
	StubValue func() T
	StubClose func() error
	StubNext  func() bool
	StubErr   func() error
}

// wrapper

func (m *StubIter[T]) Close() error {
	return m.StubClose()
}

func (m *StubIter[T]) Next() bool {
	return m.StubNext()
}

func (m *StubIter[T]) Err() error {
	return m.StubErr()
}

func (m *StubIter[T]) Value() T {
	return m.StubValue()
}

// Reseting stubs

func (m *StubIter[T]) ResetClose() {
	m.StubClose = m.Iterator.Close
}

func (m *StubIter[T]) ResetNext() {
	m.StubNext = m.Iterator.Next
}

func (m *StubIter[T]) ResetErr() {
	m.StubErr = m.Iterator.Err
}

func (m *StubIter[T]) ResetValue() {
	m.StubValue = m.Iterator.Value
}
