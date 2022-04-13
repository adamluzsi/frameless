package iterators

import "github.com/adamluzsi/frameless"

func NewMock[T any](i frameless.Iterator[T]) *Mock[T] {
	return &Mock[T]{
		Iterator:  i,
		StubValue: i.Value,
		StubClose: i.Close,
		StubNext:  i.Next,
		StubErr:   i.Err,
	}
}

type Mock[T any] struct {
	Iterator  frameless.Iterator[T]
	StubValue func() T
	StubClose func() error
	StubNext  func() bool
	StubErr   func() error
}

// wrapper

func (m *Mock[T]) Close() error {
	return m.StubClose()
}

func (m *Mock[T]) Next() bool {
	return m.StubNext()
}

func (m *Mock[T]) Err() error {
	return m.StubErr()
}

func (m *Mock[T]) Value() T {
	return m.StubValue()
}

// Reseting stubs

func (m *Mock[T]) ResetClose() {
	m.StubClose = m.Iterator.Close
}

func (m *Mock[T]) ResetNext() {
	m.StubNext = m.Iterator.Next
}

func (m *Mock[T]) ResetErr() {
	m.StubErr = m.Iterator.Err
}

func (m *Mock[T]) ResetValue() {
	m.StubValue = m.Iterator.Value
}
