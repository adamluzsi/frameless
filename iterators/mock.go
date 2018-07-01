package iterators

import (
	"github.com/adamluzsi/frameless"
)

func NewMock(i frameless.Iterator) *Mock {
	return &Mock{
		iterator: i,

		StubDecode: i.Decode,
		StubClose:  i.Close,
		StubNext:   i.Next,
		StubErr:    i.Err,
	}
}

type Mock struct {
	iterator frameless.Iterator

	StubDecode func(interface{}) error
	StubClose  func() error
	StubNext   func() bool
	StubErr    func() error
}

// wrapper

func (m *Mock) Close() error {
	return m.StubClose()
}

func (m *Mock) Next() bool {
	return m.StubNext()
}

func (m *Mock) Err() error {
	return m.StubErr()
}

func (m *Mock) Decode(i interface{}) error {
	return m.StubDecode(i)
}

// Reseting stubs

func (m *Mock) ResetClose() {
	m.StubClose = m.iterator.Close
}

func (m *Mock) ResetNext() {
	m.StubNext = m.iterator.Next
}

func (m *Mock) ResetErr() {
	m.StubErr = m.iterator.Err
}

func (m *Mock) ResetDecode() {
	m.StubDecode = m.iterator.Decode
}
